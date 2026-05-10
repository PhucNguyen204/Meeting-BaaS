// Command api-server is the public REST API for queueing bot jobs and
// dispatching webhooks. Phase 3 deliverable; this main is a stub so the
// module compiles and the binary exists in CI from day one.
package main

import (
	"context"
	"fmt"
	"os"

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
		os.Exit(1)
	}
	if err := a.Run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "api-server: %v\n", err)
		os.Exit(1)
	}
}
