// Package app is the composition root for the bot's runtime processes.
//
// Each *App struct gathers the full set of dependencies a single binary
// needs (logger, config, browser driver, meeting provider, http server,
// ...) and exposes Start / Stop. main() in cmd/<binary> just constructs
// and runs the App.
package app

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	httph "github.com/PhucNguyen204/Meeting-BaaS/internal/api/http"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/config"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/domain"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/infra/browser"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/infra/dialog"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/infra/meeting/meet"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/infra/recorder"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/infra/snapshot"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/infra/speaker"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/infra/webhook"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/pkg/logger"
	sm "github.com/PhucNguyen204/Meeting-BaaS/internal/usecase/bot"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/usecase/bot/states"
)

// BotWorker is the runtime container for the bot-worker process.
//
// One instance per pod; one pod per meeting session.
type BotWorker struct {
	Cfg            *config.BotConfig
	Logger         *zap.Logger
	BrowserDriver  domain.BrowserDriver
	DialogObserver *dialog.Observer
	Snapshot       *snapshot.Service
	Speakers       *speaker.Manager
	HTTP           *httph.Server

	// HTTPAddr is the address the control-plane HTTP server listens on.
	HTTPAddr string

	// Machine is the state machine driving the bot lifecycle.
	Machine *sm.Machine

	// stopFn is set during Run; calling it triggers graceful shutdown.
	stopFn func()
}

// BotWorkerOptions tunes how NewBotWorker constructs the App.
type BotWorkerOptions struct {
	HTTPAddr    string
	BrowserOpts domain.BrowserLaunchOptions
}

// NewBotWorker builds the dependency graph from cfg + logger.
//
// The Browser driver is NOT launched yet ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â‚¬Å¡Ã‚Â¬ÃƒÂ¢Ã¢â€šÂ¬Ã‚Â the InitializationState does that.
func NewBotWorker(cfg *config.BotConfig, log *zap.Logger, opts BotWorkerOptions) (*BotWorker, error) {
	if cfg == nil {
		return nil, fmt.Errorf("app: nil config")
	}
	if log == nil {
		return nil, fmt.Errorf("app: nil logger")
	}

	driver := browser.NewPlaywrightDriver(log)
	prov := meet.NewProvider(log)
	obs := dialog.New(log)
	snap := snapshot.New(log, "")
	speakers := speaker.NewManager(log)

	httpAddr := opts.HTTPAddr
	if httpAddr == "" {
		httpAddr = ":8080"
	}

	// Build recorder.
	recordingDir := cfg.LocalRecordingServerLocation
	if recordingDir == "" {
		recordingDir = "/tmp/recordings"
	}
	outputPath := fmt.Sprintf("%s/%s.mp4", recordingDir, cfg.BotUUID)
	rec := recorder.New(log, recorder.Options{
		OutputPath: outputPath,
	})

	// Build webhook sender.
	var wh states.Webhooker
	if cfg.WebhookURL != "" {
		sender := webhook.NewSender(log, webhook.SenderOptions{})
		wh = webhook.NewBotWebhooker(sender, cfg.WebhookURL)
	}

	// Build state map using Composition Root pattern.
	stateMap := states.BuildStateMap(states.Dependencies{
		Driver:      driver,
		Provider:    prov,
		BrowserOpts: opts.BrowserOpts,
		Recorder:    rec,
		Uploader:    nil, // S3 wired when env vars present
		Webhooker:   wh,
	})

	// Build meeting context.
	mc := &sm.MeetingContext{
		Config: cfg,
	}

	// Build machine.
	machine := sm.NewMachine(mc, stateMap, log)

	return &BotWorker{
		Cfg:            cfg,
		Logger:         log,
		BrowserDriver:  driver,
		DialogObserver: obs,
		Snapshot:       snap,
		Speakers:       speakers,
		HTTPAddr:       httpAddr,
		Machine:        machine,
	}, nil
}

// Run launches the bot. Blocks until the state machine terminates or
// ctx is cancelled.
func (a *BotWorker) Run(ctx context.Context, opts BotWorkerOptions) error {
	ctx = logger.IntoContext(ctx, a.Logger)
	log := logger.FromContext(ctx)

	log.Info("bot-worker starting",
		zap.String("bot_uuid", a.Cfg.BotUUID),
		zap.String("meeting_url", a.Cfg.MeetingURL),
		zap.Object("config", a.Cfg),
	)

	if err := a.Cfg.Validate(); err != nil {
		return fmt.Errorf("app: validate config: %w", err)
	}

	// HTTP control plane (background).
	machineStopper := &machineStopper{machine: a.Machine}
	a.HTTP = httph.New(a.HTTPAddr, a.Logger, machineStopper)
	a.HTTP.SetStatusProvider(&machineStatusProvider{machine: a.Machine})
	a.HTTP.SetPauseResumer(&machinePauseResumer{machine: a.Machine})

	httpCtx, cancelHTTP := context.WithCancel(ctx)
	defer cancelHTTP()
	go func() {
		if err := a.HTTP.Start(httpCtx); err != nil && err != context.Canceled {
			log.Warn("http server stopped", zap.Error(err))
		}
	}()

	// Run the state machine. This drives the entire bot lifecycle:
	// Initialization ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã‚Â ÃƒÂ¢Ã¢â€šÂ¬Ã¢â€žÂ¢ WaitingRoom ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã‚Â ÃƒÂ¢Ã¢â€šÂ¬Ã¢â€žÂ¢ InCall ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã‚Â ÃƒÂ¢Ã¢â€šÂ¬Ã¢â€žÂ¢ Recording ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã‚Â ÃƒÂ¢Ã¢â€šÂ¬Ã¢â€žÂ¢ Cleanup ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã‚Â ÃƒÂ¢Ã¢â€šÂ¬Ã¢â€žÂ¢ Terminated
	log.Info("starting state machine")
	if err := a.Machine.Run(ctx); err != nil {
		log.Error("state machine error", zap.Error(err))
		return err
	}

	log.Info("bot-worker completed",
		zap.Bool("successful", a.Machine.WasRecordingSuccessful()),
	)
	return nil
}

// --- Adapters wiring Machine to HTTP server interfaces ---

// machineStopper implements httph.Stopper by delegating to Machine.RequestStop.
type machineStopper struct {
	machine *sm.Machine
}

func (s *machineStopper) Stop(_ context.Context, reason string) error {
	s.machine.RequestStop(sm.EndReasonApiRequest)
	_ = reason
	return nil
}

// machineStatusProvider implements httph.StatusProvider.
type machineStatusProvider struct {
	machine *sm.Machine
}

func (p *machineStatusProvider) CurrentState() string {
	return string(p.machine.CurrentState())
}

func (p *machineStatusProvider) StartTime() int64  { return 0 }
func (p *machineStatusProvider) IsPaused() bool    { return false }
func (p *machineStatusProvider) EndReason() string { return "" }

// machinePauseResumer implements httph.PauseResumer.
type machinePauseResumer struct {
	machine *sm.Machine
}

func (pr *machinePauseResumer) Pause(_ context.Context) error {
	// TODO: wire to MeetingContext.IsPaused
	return fmt.Errorf("pause not yet wired to state machine")
}

func (pr *machinePauseResumer) Resume(_ context.Context) error {
	// TODO: wire to MeetingContext.IsPaused
	return fmt.Errorf("resume not yet wired to state machine")
}
