// Command api-server exposes the public REST API for queueing bot jobs:
//
//	GET    /healthz
//	POST   /v1/bots          — insert into Postgres + enqueue Redis Streams
//	GET    /v1/bots/{id}     — fetch bot row
//	POST   /v1/bots/{id}/stop — publish stop signal
//
// Configuration is via environment variables:
//
//	HTTP_ADDR        listen address (default :8080)
//	POSTGRES_DSN     pgx connection string (required for /v1/bots)
//	REDIS_ADDR       redis host:port (default localhost:6379)
//	REDIS_PASSWORD   optional
//	QUEUE_STREAM     (default bots:jobs)
//	LOG_LEVEL        debug|info|warn|error
package main

import (
	"context"
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
		cfg.Service = "api-server"
	}
	log, err := logger.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "api-server: logger: %v\n", err)
		os.Exit(2)
	}
	defer func() { _ = log.Sync() }()

	a, err := app.NewAPIServer(log)
	if err != nil {
		fmt.Fprintf(os.Stderr, "api-server: setup: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := a.Run(ctx); err != nil && err != context.Canceled {
		fmt.Fprintf(os.Stderr, "api-server: %v\n", err)
		os.Exit(1)
	}
}
