//go:build integration
// +build integration

// Run with: go test -tags=integration ./internal/infra/queue/...
//
// Requires Docker. Spins up a Redis container and exercises the Producer ↔
// Consumer round trip plus the Pub/Sub stop signal helper.
package queue_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"go.uber.org/zap/zaptest"

	"github.com/PhucNguyen204/Meeting-BaaS/internal/infra/queue"
)

// TestRedisStreamRoundTrip verifies XADD → XREADGROUP → XACK against a real
// Redis container.
func TestRedisStreamRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	redisC, err := tcredis.Run(ctx, "redis:7-alpine")
	if err != nil {
		t.Fatalf("redis testcontainer: %v", err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(redisC) })

	addr, err := redisC.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("redis connection string: %v", err)
	}
	// strip the redis:// prefix so go-redis can parse it as host:port.
	if u, err := goredis.ParseURL(addr); err == nil {
		addr = u.Addr
	}

	rdb := goredis.NewClient(&goredis.Options{Addr: addr})
	t.Cleanup(func() { _ = rdb.Close() })

	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Fatalf("ping: %v", err)
	}

	const stream = "bots:jobs:test"
	const group = "controller-test"

	prod := queue.NewProducer(zaptest.NewLogger(t), rdb, stream)
	cons, err := queue.NewConsumer(ctx, zaptest.NewLogger(t), rdb, queue.ConsumerOptions{
		Stream:   stream,
		Group:    group,
		Consumer: "consumer-1",
	})
	if err != nil {
		t.Fatalf("new consumer: %v", err)
	}

	cfg, _ := json.Marshal(map[string]any{"meeting_url": "https://meet.google.com/x"})
	job := queue.Job{
		BotID:     "11111111-1111-1111-1111-111111111111",
		BotUUID:   "22222222-2222-2222-2222-222222222222",
		BotConfig: cfg,
	}

	id, err := prod.Enqueue(ctx, job)
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if id == "" {
		t.Fatal("enqueue returned empty id")
	}

	got, streamID, err := cons.ReadOne(ctx, 5*time.Second)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got == nil {
		t.Fatal("expected a job, got nil")
	}
	if got.BotID != job.BotID {
		t.Fatalf("BotID: want %q got %q", job.BotID, got.BotID)
	}
	if got.BotUUID != job.BotUUID {
		t.Fatalf("BotUUID: want %q got %q", job.BotUUID, got.BotUUID)
	}

	if err := cons.Ack(ctx, streamID); err != nil {
		t.Fatalf("ack: %v", err)
	}

	// After ack the message should no longer be pending.
	pending, err := rdb.XPending(ctx, stream, group).Result()
	if err != nil {
		t.Fatalf("xpending: %v", err)
	}
	if pending.Count != 0 {
		t.Fatalf("expected 0 pending after ack, got %d", pending.Count)
	}
}

// TestPublishStopFanout asserts PublishStop reaches a subscriber on the
// bot:stop:<uuid> channel.
func TestPublishStopFanout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	redisC, err := tcredis.Run(ctx, "redis:7-alpine")
	if err != nil {
		t.Fatalf("redis testcontainer: %v", err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(redisC) })

	addr, err := redisC.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("redis connection string: %v", err)
	}
	if u, err := goredis.ParseURL(addr); err == nil {
		addr = u.Addr
	}
	rdb := goredis.NewClient(&goredis.Options{Addr: addr})
	t.Cleanup(func() { _ = rdb.Close() })

	const botUUID = "abcd-stop-uuid"
	pubsub := rdb.Subscribe(ctx, "bot:stop:"+botUUID)
	t.Cleanup(func() { _ = pubsub.Close() })

	if _, err := pubsub.Receive(ctx); err != nil {
		t.Fatalf("subscribe receive: %v", err)
	}

	if err := queue.PublishStop(ctx, rdb, botUUID, "apiRequest"); err != nil {
		t.Fatalf("publish: %v", err)
	}

	select {
	case msg := <-pubsub.Channel():
		if msg.Payload != "apiRequest" {
			t.Fatalf("payload: want apiRequest, got %q", msg.Payload)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("did not receive stop signal in time")
	}
}
