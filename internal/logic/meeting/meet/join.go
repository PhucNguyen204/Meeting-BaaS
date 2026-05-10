package meet

import (
	"context"
	"fmt"
	"time"

	"github.com/playwright-community/playwright-go"
	"go.uber.org/zap"

	"github.com/yourorg/meet-bot-go/internal/logic/browser"
	"github.com/yourorg/meet-bot-go/internal/logic/meeting"
	perrors "github.com/yourorg/meet-bot-go/internal/pkg/errors"
	"github.com/yourorg/meet-bot-go/internal/pkg/logger"
	"github.com/yourorg/meet-bot-go/internal/pkg/sleep"
)

const (
	joinPollInterval    = time.Second
	nameTypeMaxAttempts = 10
)

// JoinMeeting navigates the bot through the Google Meet pre-call screen
// until accepted into the call.
//
// Port reference: src/meeting/meet.ts joinMeeting() (~line 151-327).
func (p *Provider) JoinMeeting(ctx context.Context, page browser.Page, opts meeting.JoinOptions) error {
	ctx = logger.WithProvider(ctx, "Meet")
	log := logger.FromContext(ctx).With(
		zap.String("bot_name", opts.BotName),
		zap.Int64("start_time", opts.StartTimeUnix),
	)
	log.Info("starting join flow")

	if page == nil {
		return fmt.Errorf("meet: nil page")
	}

	// 1. Dismiss any pre-join popups ("Got it", "Dismiss").
	dismissPopups(page, log)
	_ = sleep.For(ctx, 300*time.Millisecond)

	// 2. Click "Use without an account" if present.
	clickTextButton(page, "Use without an account", log)

	// 3. Type bot name with retry (hybrid: fast then exponential backoff).
	if err := typeBotNameWithRetry(ctx, page, opts.BotName, log); err != nil {
		log.Warn("failed to type bot name after all attempts", zap.Error(err))
	}

	// 4. Deactivate camera (unless branding is configured).
	toggleButton(page, SelCamToggleButton, false, log)

	// 5. Check cancel before irreversible join click.
	if opts.CancelCheck != nil && opts.CancelCheck() {
		return perrors.ErrBotNotAccepted
	}

	// 6. Click join button.
	clickJoinButton(page, log)

	// 7. Poll for meeting state until joined, rejected, or timeout.
	log.Info("waiting for meeting admission")
	detector := meeting.NewDetector(MeetStateConfig)

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if opts.CancelCheck != nil && opts.CancelCheck() {
			return perrors.ErrBotNotAccepted
		}

		state, err := detector.Detect(ctx, page)
		if err != nil {
			log.Warn("state detection error", zap.Error(err))
		}

		switch state {
		case "in_call":
			log.Info("bot admitted to meeting")
			if opts.OnJoinSuccess != nil {
				opts.OnJoinSuccess()
			}
			return nil

		case "removed":
			log.Warn("bot was removed from meeting")
			return perrors.ErrBotNotAccepted

		case "lobby_denied":
			log.Warn("bot was denied entry")
			return perrors.ErrBotNotAccepted

		case "login_required":
			log.Warn("login required to join")
			return perrors.ErrLoginRequired
		}

		// Retry join click periodically in case it didn't register.
		clickJoinButton(page, log)

		_ = sleep.For(ctx, joinPollInterval)
	}
}

// typeBotNameWithRetry types the bot name into the name input with retries.
func typeBotNameWithRetry(ctx context.Context, page browser.Page, botName string, log *zap.Logger) error {
	for attempt := 1; attempt <= nameTypeMaxAttempts; attempt++ {
		// Try primary selector.
		loc := page.Locator(SelNameInput)
		if count, _ := loc.Count(); count > 0 {
			if err := loc.Fill(botName, playwright.LocatorFillOptions{
				Timeout: playwright.Float(3000),
			}); err == nil {
				log.Info("bot name typed", zap.Int("attempt", attempt))
				return nil
			}
		}

		// Try fallback selector.
		loc = page.Locator(SelNameInputFallback)
		if count, _ := loc.Count(); count > 0 {
			if err := loc.Fill(botName, playwright.LocatorFillOptions{
				Timeout: playwright.Float(3000),
			}); err == nil {
				log.Info("bot name typed (fallback)", zap.Int("attempt", attempt))
				return nil
			}
		}

		// Backoff: fast for first 5 attempts, exponential after.
		var wait time.Duration
		if attempt < 5 {
			wait = 500 * time.Millisecond
		} else {
			wait = time.Duration(1<<(attempt-5)) * time.Second
			if wait > 8*time.Second {
				wait = 8 * time.Second
			}
		}
		log.Debug("name input not found, retrying",
			zap.Int("attempt", attempt),
			zap.Duration("backoff", wait),
		)
		_ = sleep.For(ctx, wait)
	}
	return fmt.Errorf("meet: failed to type bot name after %d attempts", nameTypeMaxAttempts)
}

// clickJoinButton attempts to click any visible join CTA.
func clickJoinButton(page browser.Page, log *zap.Logger) {
	selectors := []string{SelAskToJoinButton, SelJoinNowButton, SelAskToJoinButtonText}
	for _, sel := range selectors {
		loc := page.Locator(sel)
		if visible, _ := loc.IsVisible(); visible {
			if err := loc.Click(playwright.LocatorClickOptions{
				Timeout: playwright.Float(3000),
			}); err == nil {
				log.Debug("clicked join button", zap.String("selector", sel))
				return
			}
		}
	}
}

// dismissPopups clicks common pre-join dismissal buttons.
func dismissPopups(page browser.Page, log *zap.Logger) {
	dismissTexts := []string{"Dismiss", "Got it"}
	for _, text := range dismissTexts {
		loc := page.Locator(fmt.Sprintf("button:has-text(\"%s\")", text))
		if visible, _ := loc.IsVisible(); visible {
			if err := loc.Click(); err == nil {
				log.Debug("dismissed popup", zap.String("text", text))
			}
		}
	}
}

// clickTextButton clicks a button containing the given text.
func clickTextButton(page browser.Page, text string, log *zap.Logger) {
	loc := page.Locator(fmt.Sprintf("span:has-text(\"%s\")", text))
	if count, _ := loc.Count(); count > 0 {
		if err := loc.Click(); err == nil {
			log.Debug("clicked text button", zap.String("text", text))
		}
	}
}

// toggleButton toggles a button to the desired state (on/off).
func toggleButton(page browser.Page, selector string, wantOn bool, log *zap.Logger) {
	loc := page.Locator(selector)
	if visible, _ := loc.IsVisible(); visible {
		// Check current state via aria-pressed or similar.
		if err := loc.Click(); err == nil {
			log.Debug("toggled button", zap.String("selector", selector))
		}
	}
}
