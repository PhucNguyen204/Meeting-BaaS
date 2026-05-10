package dialog

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/playwright-community/playwright-go"
	"go.uber.org/zap"

	"github.com/PhucNguyen204/Meeting-BaaS/internal/domain"
)

// Observer watches a Page for popup dialogs and dispatches matching events
// to subscribed handlers, while attempting to auto-dismiss known modals
// (recording notice, camera permission, privacy banner, …).
//
// Port reference: src/services/dialog-observer/simple-dialog-observer.ts.
//
// Approach (mirrors TS):
//   - A goroutine ticks every 2s.
//   - On each tick: walk a list of (selector, button-texts) patterns; for the
//     first visible match, try clicking each button-text in turn; if none
//     dismiss and the pattern allows Escape, press Escape.
//   - Each detected dialog is also dispatched to subscribed Handlers.
//
// Construct one Observer per page. Safe for concurrent Subscribe / Unsubscribe.
type Observer struct {
	log *zap.Logger

	mu       sync.RWMutex
	handlers []Handler
	paused   bool
	page     domain.Page
	cancel   context.CancelFunc
}

// New returns a fresh observer. Call Attach(page) to start the polling loop.
func New(log *zap.Logger) *Observer {
	if log == nil {
		log = zap.NewNop()
	}
	return &Observer{log: log.Named("dialog")}
}

// Subscribe adds a handler. Handlers are invoked in the order they were
// registered. Pass nil to no-op.
func (o *Observer) Subscribe(h Handler) {
	if h == nil {
		return
	}
	o.mu.Lock()
	o.handlers = append(o.handlers, h)
	o.mu.Unlock()
}

// Pause and Resume temporarily disable / re-enable the polling cycle.
//
// Port reference: src/services/dialog-observer/simple-dialog-observer.ts
// SimpleDialogObserver.pause / resume.
func (o *Observer) Pause() {
	o.mu.Lock()
	o.paused = true
	o.mu.Unlock()
}
func (o *Observer) Resume() {
	o.mu.Lock()
	o.paused = false
	o.mu.Unlock()
}

// Attach binds the observer to page and spawns a goroutine that polls for
// dialogs every 2 seconds. Idempotent — subsequent calls swap the page.
func (o *Observer) Attach(ctx context.Context, page domain.Page) error {
	if page == nil || o == nil {
		return nil
	}
	o.log.Info("attaching dialog observer")

	o.mu.Lock()
	if o.cancel != nil {
		o.cancel() // tear down previous loop
	}
	o.page = page
	loopCtx, cancel := context.WithCancel(ctx)
	o.cancel = cancel
	o.mu.Unlock()

	go o.runLoop(loopCtx)
	return nil
}

// Stop terminates the polling loop. Safe to call multiple times.
func (o *Observer) Stop() {
	o.mu.Lock()
	if o.cancel != nil {
		o.cancel()
		o.cancel = nil
	}
	o.mu.Unlock()
}

func (o *Observer) runLoop(ctx context.Context) {
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			o.mu.RLock()
			paused := o.paused
			page := o.page
			o.mu.RUnlock()
			if paused || page == nil {
				continue
			}
			o.dismissOnce(ctx, page)
		}
	}
}

// dialogPattern describes a known modal we want to auto-dismiss.
//
// Port reference: simple-dialog-observer.ts modalPatterns array.
type dialogPattern struct {
	Name          string
	Selector      string
	ButtonTexts   []string
	ExitByEscape  bool
	Severity      Severity
}

var meetDialogPatterns = []dialogPattern{
	{Name: "people_hover_dialog", Selector: `div[role="dialog"][aria-label*="people in the call" i]`, ExitByEscape: true, Severity: SeverityInfo},
	{Name: "recording_notification", Selector: `div[role="dialog"]:has-text("video call is being recorded")`, ButtonTexts: []string{"Join now"}, Severity: SeverityWarn},
	{Name: "transcribe_notification", Selector: `div[role="dialog"]:has-text("video call is being transcribed")`, ButtonTexts: []string{"Join now"}, Severity: SeverityWarn},
	{Name: "gemini_notification", Selector: `div[role="dialog"]:has-text("Gemini"):has-text("taking notes")`, ButtonTexts: []string{"Join now"}, Severity: SeverityInfo},
	{Name: "privacy_notification", Selector: `div[role="dialog"]:has-text("Others may see")`, ButtonTexts: []string{"Got it", "OK", "Dismiss", "Close"}, Severity: SeverityInfo},
	{Name: "video_privacy", Selector: `div[role="dialog"]:has-text("video differently")`, ButtonTexts: []string{"Got it", "OK", "Continue"}, Severity: SeverityInfo},
	{Name: "background_feed", Selector: `div[role="dialog"]:has-text("background")`, ButtonTexts: []string{"Got it", "OK", "Dismiss"}, Severity: SeverityInfo},
	{Name: "camera_permission", Selector: `div[role="dialog"]:has-text("camera")`, ButtonTexts: []string{"Allow", "Block", "Got it", "OK", "Join now"}, ExitByEscape: true, Severity: SeverityInfo},
	{Name: "generic_dismiss", Selector: `div[role="dialog"]:has(button)`, ButtonTexts: []string{"Join now", "Got it", "OK", "Dismiss", "Close", "Continue"}, Severity: SeverityInfo},
}

// dismissOnce performs one detect-and-dismiss cycle. Returns the first
// matching pattern (or empty if none). Errors are logged at debug level.
func (o *Observer) dismissOnce(_ context.Context, page domain.Page) string {
	visibleTimeout := float64(500)  // ms
	clickTimeout := float64(1000)   // ms

	for _, pat := range meetDialogPatterns {
		modal := page.Locator(pat.Selector)
		visible, err := modal.IsVisible(playwright.LocatorIsVisibleOptions{Timeout: &visibleTimeout})
		if err != nil || !visible {
			continue
		}

		o.log.Debug("dialog detected", zap.String("pattern", pat.Name))
		title := pat.Name
		body, _ := modal.TextContent()
		auto := o.dispatch(Event{
			Title:    title,
			Body:     body,
			Selector: pat.Selector,
			Severity: pat.Severity,
			At:       time.Now(),
		})

		dismissed := false
		for _, btnText := range pat.ButtonTexts {
			if o.tryClickButton(modal, btnText, clickTimeout) {
				dismissed = true
				break
			}
		}
		if !dismissed && (pat.ExitByEscape || auto) {
			if err := page.Keyboard().Press("Escape"); err == nil {
				dismissed = true
			}
		}
		if dismissed {
			o.log.Info("dialog dismissed",
				zap.String("pattern", pat.Name),
				zap.Bool("auto_handler", auto),
			)
			return pat.Name
		}
		// First visible pattern wins; don't iterate further.
		return pat.Name
	}
	return ""
}

// tryClickButton attempts several text-matching strategies for a given button
// text inside the modal locator. Returns true on first successful click.
func (o *Observer) tryClickButton(modal domain.Locator, text string, clickTimeout float64) bool {
	visibleTimeout := float64(500)

	candidates := []string{
		`button:has-text("` + text + `")`,
		`button:text-matches(".*` + text + `.*", "i")`,
		`button span:has-text("` + text + `")`,
		`button[aria-label="` + text + `" i]`,
	}
	for _, sel := range candidates {
		btn := modal.Locator(sel).First()
		visible, err := btn.IsVisible(playwright.LocatorIsVisibleOptions{Timeout: &visibleTimeout})
		if err != nil || !visible {
			continue
		}
		// Use evaluate-based click to bypass actionability checks (parity with TS).
		if _, err := btn.Evaluate(`(el) => el.click()`, nil); err == nil {
			return true
		}
		// Fallback to native click.
		if err := btn.Click(playwright.LocatorClickOptions{Timeout: &clickTimeout}); err == nil {
			return true
		}
	}
	// span match needs xpath=.. to find parent button.
	if strings.Contains(text, " ") {
		return false
	}
	parent := modal.Locator(`button span:has-text("` + text + `")`).First().Locator(`xpath=..`)
	visible, _ := parent.IsVisible(playwright.LocatorIsVisibleOptions{Timeout: &visibleTimeout})
	if !visible {
		return false
	}
	if _, err := parent.Evaluate(`(el) => el.click()`, nil); err == nil {
		return true
	}
	return false
}

// dispatch calls every handler. If any handler returns autoDismiss=true the
// observer falls back to the Escape key after button-clicks fail.
func (o *Observer) dispatch(ev Event) (autoDismiss bool) {
	o.mu.RLock()
	hs := append([]Handler(nil), o.handlers...)
	o.mu.RUnlock()
	for _, h := range hs {
		if h(ev) {
			autoDismiss = true
		}
	}
	return autoDismiss
}
