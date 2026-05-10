// Package states contains the concrete State implementations for the
// meeting bot lifecycle.
//
// Each file corresponds to one state. All states satisfy statemachine.State.
package states

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/yourorg/meet-bot-go/internal/config"
	"github.com/yourorg/meet-bot-go/internal/logic/browser"
	"github.com/yourorg/meet-bot-go/internal/logic/meeting"
	sm "github.com/yourorg/meet-bot-go/internal/logic/statemachine"
	"github.com/yourorg/meet-bot-go/internal/pkg/logger"
	"github.com/yourorg/meet-bot-go/internal/pkg/retry"
)

// InitializationState handles browser launch, URL parsing, provider
// detection, and page navigation.
//
// Port reference: src/state-machine/states/initialization-state.ts.
type InitializationState struct {
	Driver   browser.Driver
	Provider meeting.Provider
	Opts     browser.LaunchOptions
}

func (s *InitializationState) Name() sm.StateType { return sm.StateInitialization }

func (s *InitializationState) Execute(ctx context.Context, mc *sm.MeetingContext) (sm.Transition, error) {
	log := logger.FromContext(ctx).With(zap.String("state", "initialization"))
	log.Info("initializing bot session",
		zap.String("meeting_url", mc.Config.MeetingURL),
		zap.String("bot_name", mc.Config.BotName),
	)

	// 1. Detect provider from URL.
	provider, info, err := meeting.ParseURL(ctx, mc.Config.MeetingURL)
	if err != nil {
		mc.SetError(sm.EndReasonInvalidMeetingURL, err.Error())
		return sm.Transition{Next: sm.StateError}, nil
	}
	mc.Config.MeetingProvider = provider
	log.Info("provider detected",
		zap.String("provider", string(provider)),
		zap.String("meeting_id", info.MeetingID),
	)

	// 2. Launch browser with retry (3 attempts, exponential backoff).
	err = retry.Do(ctx, retry.Options{MaxAttempts: 3}, func(ctx context.Context, _ int) error {
		return s.Driver.Launch(ctx, s.Opts)
	})
	if err != nil {
		mc.SetError(sm.EndReasonCannotJoinMeeting, fmt.Sprintf("browser launch failed: %v", err))
		return sm.Transition{Next: sm.StateError}, nil
	}
	mc.BrowserDriver = s.Driver

	// 3. Open meeting page.
	link := s.Provider.BuildMeetingLink(info, 0, mc.Config.BotName, mc.Config.EnterMessage)
	page, err := s.Provider.OpenMeetingPage(ctx, s.Driver.Context(), link, mc.Config.StreamingInput)
	if err != nil {
		mc.SetError(sm.EndReasonCannotJoinMeeting, fmt.Sprintf("open page failed: %v", err))
		return sm.Transition{Next: sm.StateError}, nil
	}
	mc.Page = page
	mc.Provider = s.Provider

	log.Info("initialization complete, transitioning to waiting room")
	return sm.Transition{Next: sm.StateWaitingRoom}, nil
}

// providerForConfig returns the meeting provider for the given config.
// Only Meet is supported in Phase 1.
func providerForConfig(cfg *config.BotConfig) (meeting.Provider, error) {
	switch cfg.MeetingProvider {
	case config.ProviderMeet:
		return nil, nil // caller provides the provider
	default:
		return nil, fmt.Errorf("unsupported provider: %s", cfg.MeetingProvider)
	}
}
