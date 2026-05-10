// Command controller consumes bot jobs from Redis Streams and dispatches them
// to bot-worker processes (Phase 3) or K8s Jobs (Phase 6).
//
// Phase 3 implementation:
//
//	XREADGROUP bots:jobs > controller:<consumer>
//	→ os/exec ./bin/bot-worker (stdin = BotConfig JSON)
//	→ wait for clean exit, then XACK
//
// Configuration via environment variables (see internal/app.ControllerOptions).
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/PhucNguyen204/Meeting-BaaS/internal/app"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/pkg/logger"
)

func main() {
	cfg := logger.ConfigFromEnv()
	if cfg.Service == "" {
		cfg.Service = "controller"
	}
	log, err := logger.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "controller: logger: %v\n", err)
		os.Exit(2)
	}
	defer func() { _ = log.Sync() }()

	c, err := app.NewController(log)
	if err != nil {
		fmt.Fprintf(os.Stderr, "controller: setup: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := c.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintf(os.Stderr, "controller: %v\n", err)
		os.Exit(1)
	}
}
