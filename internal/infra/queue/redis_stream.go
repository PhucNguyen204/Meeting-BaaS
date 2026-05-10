// Package queue provides a Redis Streams-based job queue for bot
// dispatch (api-server → controller). The controller then spawns the actual
// bot-worker process or K8s Job.
//
// Phase 3. Stream key default: "bots:jobs". Consumer group default: "controller".
package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Job is the payload pushed onto the stream by api-server. It is JSON-serialised
// to a single field "payload" so any consumer can decode it without negotiating
// an XADD field schema.
//
// The BotConfig itself is opaque from the queue's POV — the controller hands it
// to bot-worker via stdin without ever decoding the inner fields.
type Job struct {
	BotID      string          `json:"bot_id"`        // bots.id (UUID)
	BotUUID    string          `json:"bot_uuid"`      // for matching pub/sub stop channel
	BotConfig  json.RawMessage `json:"bot_config"`    // raw BotConfig JSON (passed to bot-worker stdin)
	EnqueuedAt time.Time       `json:"enqueued_at"`
}

// DefaultStream / DefaultGroup constants.
const (
	DefaultStream = "bots:jobs"
	DefaultGroup  = "controller"
)

// Producer pushes Jobs onto the stream.
type Producer struct {
	rdb    *redis.Client
	stream string
	log    *zap.Logger
}

// NewProducer constructs a producer bound to a stream key (defaults DefaultStream).
func NewProducer(log *zap.Logger, rdb *redis.Client, stream string) *Producer {
	if log == nil {
		log = zap.NewNop()
	}
	if stream == "" {
		stream = DefaultStream
	}
	return &Producer{rdb: rdb, stream: stream, log: log.Named("queue.producer")}
}

// Enqueue serialises job to JSON and XADDs it. Returns the assigned stream id.
func (p *Producer) Enqueue(ctx context.Context, job Job) (string, error) {
	if p == nil || p.rdb == nil {
		return "", errors.New("queue: nil producer")
	}
	if job.EnqueuedAt.IsZero() {
		job.EnqueuedAt = time.Now()
	}
	body, err := json.Marshal(job)
	if err != nil {
		return "", fmt.Errorf("queue: marshal job: %w", err)
	}
	id, err := p.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: p.stream,
		Values: map[string]any{"payload": body},
	}).Result()
	if err != nil {
		return "", fmt.Errorf("queue: xadd: %w", err)
	}
	p.log.Info("job enqueued",
		zap.String("stream", p.stream),
		zap.String("id", id),
		zap.String("bot_id", job.BotID),
	)
	return id, nil
}

// Consumer reads Jobs from the stream and acknowledges them.
type Consumer struct {
	rdb      *redis.Client
	stream   string
	group    string
	consumer string
	log      *zap.Logger
}

// ConsumerOptions configures the consumer.
type ConsumerOptions struct {
	Stream   string // default DefaultStream
	Group    string // default DefaultGroup
	Consumer string // unique consumer name within the group; required
}

// NewConsumer constructs a consumer and ensures the consumer group exists.
//
// If the group does not exist, it is created with MKSTREAM and starting at $.
// (i.e. only new messages — the controller is expected to be online before
// api-server XADDs.)
func NewConsumer(ctx context.Context, log *zap.Logger, rdb *redis.Client, opts ConsumerOptions) (*Consumer, error) {
	if log == nil {
		log = zap.NewNop()
	}
	if opts.Stream == "" {
		opts.Stream = DefaultStream
	}
	if opts.Group == "" {
		opts.Group = DefaultGroup
	}
	if opts.Consumer == "" {
		return nil, errors.New("queue: ConsumerOptions.Consumer required")
	}

	// XGROUP CREATE … MKSTREAM is idempotent on first run; subsequent runs
	// return BUSYGROUP which we treat as success.
	_, err := rdb.XGroupCreateMkStream(ctx, opts.Stream, opts.Group, "$").Result()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return nil, fmt.Errorf("queue: xgroup create: %w", err)
	}

	return &Consumer{
		rdb:      rdb,
		stream:   opts.Stream,
		group:    opts.Group,
		consumer: opts.Consumer,
		log:      log.Named("queue.consumer"),
	}, nil
}

// ReadOne blocks (up to block) for a single message from the stream and returns
// the parsed Job + the stream id used for Ack. Returns (nil, "", nil) on timeout.
func (c *Consumer) ReadOne(ctx context.Context, block time.Duration) (*Job, string, error) {
	if c == nil || c.rdb == nil {
		return nil, "", errors.New("queue: nil consumer")
	}
	if block <= 0 {
		block = 5 * time.Second
	}
	res, err := c.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    c.group,
		Consumer: c.consumer,
		Streams:  []string{c.stream, ">"},
		Count:    1,
		Block:    block,
	}).Result()
	if errors.Is(err, redis.Nil) {
		return nil, "", nil // timeout, no jobs
	}
	if err != nil {
		return nil, "", fmt.Errorf("queue: xreadgroup: %w", err)
	}
	if len(res) == 0 || len(res[0].Messages) == 0 {
		return nil, "", nil
	}
	msg := res[0].Messages[0]
	raw, _ := msg.Values["payload"].(string)
	var job Job
	if err := json.Unmarshal([]byte(raw), &job); err != nil {
		c.log.Warn("malformed job, acking and dropping",
			zap.String("id", msg.ID),
			zap.Error(err),
		)
		_, _ = c.rdb.XAck(ctx, c.stream, c.group, msg.ID).Result()
		return nil, "", fmt.Errorf("queue: unmarshal job: %w", err)
	}
	return &job, msg.ID, nil
}

// Ack confirms a message was processed.
func (c *Consumer) Ack(ctx context.Context, streamID string) error {
	if c == nil || c.rdb == nil {
		return errors.New("queue: nil consumer")
	}
	_, err := c.rdb.XAck(ctx, c.stream, c.group, streamID).Result()
	if err != nil {
		return fmt.Errorf("queue: xack: %w", err)
	}
	return nil
}

// PublishStop sends a Pub/Sub message to the bot's stop channel.
//
// Format mirrors the existing storage/redis.Client.PublishStopSignal so that
// a bot-worker subscribed via Client.SubscribeStopSignal gets the message.
func PublishStop(ctx context.Context, rdb *redis.Client, botUUID, reason string) error {
	channel := fmt.Sprintf("bot:stop:%s", botUUID)
	return rdb.Publish(ctx, channel, reason).Err()
}
