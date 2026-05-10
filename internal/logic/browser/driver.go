package browser

import (
	"context"
	"time"
)

// LaunchOptions configures the persistent Chromium context.
//
// All fields have defaults — pass a zero LaunchOptions{} to get the
// recommended setup for a bot pod.
type LaunchOptions struct {
	// ChromePath overrides the executable. Empty defaults to $CHROME_PATH or
	// /usr/bin/google-chrome (Linux container).
	ChromePath string

	// Headless toggles --headless. In production we run with Xvfb, never
	// --headless, because Meet's WebRTC stack misbehaves headless.
	Headless bool

	// Resolution must be "720" or "1080". Empty defaults to "720".
	Resolution string

	// SlowMoMs slows every Playwright action by this many milliseconds.
	// Useful while debugging selectors. Zero = disabled.
	SlowMoMs int

	// LaunchTimeout caps how long Launch will block before failing.
	// Zero defaults to 60s.
	LaunchTimeout time.Duration

	// Locale sets context locale + Accept-Language. Empty defaults "en-US".
	Locale string

	// PermissionsMicrophone grants the microphone permission to all origins.
	// Required when streaming_input is configured (TS does the same).
	PermissionsMicrophone bool
}

// Driver abstracts a Playwright runtime + persistent context.
//
// Lifecycle:
//
//	d := NewPlaywrightDriver(log)
//	if err := d.Launch(ctx, opts); err != nil { ... }
//	defer d.Close(ctx)
//	page, _ := d.NewPage(ctx)
//	// use page ...
//
// Implementations must be safe to call Launch exactly once. Subsequent
// Launch calls return an error.
type Driver interface {
	// Launch starts the Playwright runtime and a persistent Chromium context.
	// Equivalent to src/browser/browser.ts:openBrowser().
	Launch(ctx context.Context, opts LaunchOptions) error

	// Context returns the persistent BrowserContext. Returns nil before Launch.
	Context() BrowserContext

	// NewPage opens a new tab inside the persistent context.
	NewPage(ctx context.Context) (Page, error)

	// Close stops Playwright and any associated processes.
	Close(ctx context.Context) error
}
