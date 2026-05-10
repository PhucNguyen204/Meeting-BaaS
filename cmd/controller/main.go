// Command controller is the Kubernetes controller that watches for new
// bot jobs on Redis Streams and creates per-bot K8s Jobs.
//
// Phase 6 deliverable. This main exists to keep the binary buildable
// alongside the others.
package main

import (
	"fmt"
	"os"

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

	log.Warn("controller is a Phase 1 stub; not implemented")
	os.Exit(0)
}
