// Command bot-worker runs a single meeting bot session.
//
// Lifecycle:
//
//		cat bot.config.json | bot-worker
//
//	 1. Reads BotConfig from stdin (or BOT_CONFIG_FILE / BOT_CONFIG_JSON env
//	    if stdin is empty).
//	 2. Builds the structured logger.
//	 3. Constructs the App graph (browser driver + meet provider + http server).
//	 4. Runs until SIGINT/SIGTERM, /stop_record, or the meeting ends.
//
// Port reference: src/main.ts (entry point).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"github.com/PhucNguyen204/Meeting-BaaS/internal/app"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/config"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/domain"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/pkg/logger"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/pkg/version"
)

func main() {
	var (
		flagConfigPath = flag.String("config", "", "path to bot config json (overrides stdin/env)")
		flagHTTPAddr   = flag.String("http-addr", ":8080", "control-plane HTTP listen address")
	)
	flag.Parse()

	cfg, err := config.Load(*flagConfigPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bot-worker: config: %v\n", err)
		os.Exit(2)
	}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "bot-worker: config invalid: %v\n", err)
		os.Exit(2)
	}

	logCfg := logger.ConfigFromEnv()
	if logCfg.BotUUID == "" {
		logCfg.BotUUID = cfg.BotUUID
	}
	if logCfg.Service == "" {
		logCfg.Service = "bot-worker"
	}
	if logCfg.Version == "" {
		logCfg.Version = version.Get().Version
	}
	if logCfg.LogDir == "" {
		logCfg.LogDir = "./logs"
	}
	log, err := logger.New(logCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bot-worker: logger: %v\n", err)
		os.Exit(2)
	}
	defer func() { _ = log.Sync() }()

	log.Info("starting bot-worker",
		zap.String("version", version.Get().Version),
		zap.String("commit", version.Get().Commit),
		zap.String("build_date", version.Get().BuildDate),
	)

	a, err := app.NewBotWorker(cfg, log, app.BotWorkerOptions{
		HTTPAddr: *flagHTTPAddr,
	})
	if err != nil {
		log.Error("construct app failed", zap.Error(err))
		os.Exit(1)
	}

	ctx, cancel := signalContext()
	defer cancel()

	runOpts := app.BotWorkerOptions{
		HTTPAddr: *flagHTTPAddr,
		BrowserOpts: domain.BrowserLaunchOptions{
			Resolution:            "720",
			Headless:              false,
			PermissionsMicrophone: cfg.StreamingInput != "",
		},
	}
	if err := a.Run(ctx, runOpts); err != nil {
		log.Error("bot-worker run failed", zap.Error(err))
		os.Exit(1)
	}
	log.Info("bot-worker exited cleanly")
}

// signalContext returns a context cancelled on SIGINT/SIGTERM.
func signalContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ch
		cancel()
	}()
	return ctx, cancel
}
