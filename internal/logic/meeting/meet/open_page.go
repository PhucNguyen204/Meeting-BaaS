package meet

import (
	"context"
	"fmt"
	"time"

	"github.com/playwright-community/playwright-go"
	"go.uber.org/zap"

	"github.com/yourorg/meet-bot-go/internal/logic/browser"
	"github.com/yourorg/meet-bot-go/internal/pkg/sleep"
)

const maxOpenPageAttempts = 3

// OpenMeetingPage navigates the Chromium context to the Google Meet URL,
// with page freeze detection and retry.
//
// Port reference: src/meeting/meet.ts openMeetingPage() (~line 57-149).
func (p *Provider) OpenMeetingPage(
	ctx context.Context,
	bctx browser.BrowserContext,
	link, streamingInput string,
) (browser.Page, error) {
	return p.openMeetingPageWithRetry(ctx, bctx, link, streamingInput, 0)
}

func (p *Provider) openMeetingPageWithRetry(
	ctx context.Context,
	bctx browser.BrowserContext,
	link, streamingInput string,
	attempt int,
) (browser.Page, error) {
	log := p.log.With(
		zap.String("link", link),
		zap.Bool("streaming", streamingInput != ""),
		zap.Int("attempt", attempt+1),
	)
	log.Info("opening meet page")

	if bctx == nil {
		return nil, fmt.Errorf("meet: nil browser context")
	}

	page, err := bctx.NewPage()
	if err != nil {
		return nil, fmt.Errorf("meet: new page: %w", err)
	}

	// Inject HTML cleaner before navigation.
	cleaner := NewHtmlCleaner()
	if err := cleaner.Inject(ctx, page); err != nil {
		log.Warn("html cleaner inject failed", zap.Error(err))
	}

	// Navigate to the meeting URL.
	resp, err := page.Goto(link, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		Timeout:   playwright.Float(30_000),
	})
	if err != nil {
		_ = page.Close()
		return nil, fmt.Errorf("meet: goto %s: %w", link, err)
	}

	// Check for HTTP 5xx from Google.
	if resp != nil && resp.Status() >= 500 {
		_ = page.Close()
		return nil, fmt.Errorf("meet: google returned HTTP %d", resp.Status())
	}

	// Page freeze detection: evaluate a simple JS expression with timeout.
	freezeDetected := false
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, evalErr := page.Evaluate("() => document.readyState")
		if evalErr != nil {
			freezeDetected = true
		}
	}()

	select {
	case <-done:
		// Evaluation completed.
	case <-time.After(10 * time.Second):
		freezeDetected = true
		log.Warn("page appears frozen after navigation")
	}

	// Retry on freeze.
	if freezeDetected && attempt < maxOpenPageAttempts-1 {
		_ = page.Close()
		log.Info("page freeze detected, retrying")
		_ = sleep.For(ctx, time.Second)
		return p.openMeetingPageWithRetry(ctx, bctx, link, streamingInput, attempt+1)
	}

	log.Info("meet page opened successfully")
	return page, nil
}
