package meet

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/playwright-community/playwright-go"

	"github.com/yourorg/meet-bot-go/internal/logic/browser"
	"github.com/yourorg/meet-bot-go/internal/pkg/sleep"
)

// CloseMeeting attempts to gracefully leave the active call.
//
// Strategy (port from src/meeting/meet/closeMeeting.ts):
//  1. Click the leave button.
//  2. Wait briefly for the page to navigate to the "you left" screen.
//  3. Errors are logged but not fatal — cleanup closes the page anyway.
func (p *Provider) CloseMeeting(ctx context.Context, page browser.Page) error {
	log := p.log.With(zap.String("op", "close"))
	if page == nil {
		log.Debug("close: nil page, skipping")
		return nil
	}
	log.Info("attempting graceful leave")

	loc := page.Locator(SelLeaveCallButton)
	if visible, _ := loc.IsVisible(); visible {
		if err := loc.Click(playwright.LocatorClickOptions{
			Timeout: playwright.Float(5000),
		}); err != nil {
			log.Warn("click leave button failed", zap.Error(err))
		} else {
			log.Info("leave button clicked")
		}
	} else {
		log.Debug("leave button not visible, may already have left")
	}

	// Wait for the page to settle.
	_ = sleep.For(ctx, 2*time.Second)

	log.Info("close meeting complete")
	return nil
}
