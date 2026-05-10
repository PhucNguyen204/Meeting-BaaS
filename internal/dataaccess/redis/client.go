// Package redis provides a Redis client for Pub/Sub stop signals and
// job queue management.
package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Client wraps go-redis for the bot's needs: stop signal subscriber
// and job queue consumer.
type Client struct {
	log    *zap.Logger
	rdb    *redis.Client
}

// Options configures the Redis client.
type Options struct {
	Addr     string // e.g. localhost:6379
	Password string
	DB       int
}

// NewClient creates a Redis client.
func NewClient(log *zap.Logger, opts Options) (*Client, error) {
	if log == nil {
		log = zap.NewNop()
	}
	if opts.Addr == "" {
		opts.Addr = "localhost:6379"
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     opts.Addr,
		Password: opts.Password,
		DB:       opts.DB,
	})

	return &Client{
		log: log.Named("redis"),
		rdb: rdb,
	}, nil
}

// Ping verifies connectivity.
func (c *Client) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return c.rdb.Ping(ctx).Err()
}

// SubscribeStopSignal subscribes to a Pub/Sub channel for bot stop signals.
// Returns a channel that receives the stop reason when a signal arrives.
//
// The caller should start this in a goroutine and select on the returned channel
// alongside the main context.
func (c *Client) SubscribeStopSignal(ctx context.Context, botUUID string) (<-chan string, error) {
	channel := fmt.Sprintf("bot:stop:%s", botUUID)
	pubsub := c.rdb.Subscribe(ctx, channel)

	// Verify subscription.
	if _, err := pubsub.Receive(ctx); err != nil {
		_ = pubsub.Close()
		return nil, fmt.Errorf("redis: subscribe %s: %w", channel, err)
	}

	out := make(chan string, 1)
	go func() {
		defer close(out)
		defer func() { _ = pubsub.Close() }()

		ch := pubsub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				c.log.Info("stop signal received",
					zap.String("channel", channel),
					zap.String("reason", msg.Payload),
				)
				select {
				case out <- msg.Payload:
				default:
				}
				return
			}
		}
	}()

	c.log.Info("subscribed to stop signal", zap.String("channel", channel))
	return out, nil
}

// PublishStopSignal publishes a stop signal for a given bot.
func (c *Client) PublishStopSignal(ctx context.Context, botUUID, reason string) error {
	channel := fmt.Sprintf("bot:stop:%s", botUUID)
	return c.rdb.Publish(ctx, channel, reason).Err()
}

// Close closes the Redis connection.
func (c *Client) Close() error {
	return c.rdb.Close()
}
