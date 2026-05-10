package browser

import (
	"context"
	"errors"
	"fmt"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/domain"
	"sync"
	"time"

	"github.com/playwright-community/playwright-go"
	"go.uber.org/zap"

	"github.com/PhucNguyen204/Meeting-BaaS/internal/pkg/logger"
)

// playwrightDriver is the production [Driver] implementation backed by
// playwright-go. It launches a persistent Chromium context so the bot
// can re-use the same profile/session across multiple pages within one
// session (matching the TS launchPersistentContext(â€) behaviour).
//
// Port reference: src/browser/browser.ts:openBrowser().
type playwrightDriver struct {
	mu      sync.Mutex
	pw      *playwright.Playwright
	context domain.BrowserContext
	log     *zap.Logger
}

// NewPlaywrightDriver constructs a domain.BrowserDriver using playwright-go.
//
// log is required (use logger.New + IntoContext to populate it).
func NewPlaywrightDriver(log *zap.Logger) domain.BrowserDriver {
	return &playwrightDriver{log: log}
}

// Launch starts Playwright + persistent Chromium context with the recommended
// args. Safe to call exactly once; subsequent calls return an error.
//
// TODO(user): port retry-on-failure logic from
// [src/state-machine/states/initialization-state.ts:68-136] which retries
// 3 times with progressive 5s/10s/15s backoff. The retry package
// (internal/pkg/retry) is ready to use.
func (d *playwrightDriver) Launch(ctx context.Context, opts domain.BrowserLaunchOptions) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.context != nil {
		return errors.New("browser: already launched")
	}

	log := d.log.With(
		zap.String("chrome_path", ResolveChromePath(opts)),
		zap.String("resolution", fallback(opts.Resolution, "720")),
		zap.Bool("headless", opts.Headless),
	)
	log.Info("launching persistent chromium context")

	pw, err := playwright.Run()
	if err != nil {
		return fmt.Errorf("browser: playwright run: %w", err)
	}

	width, height := ViewportPixels(opts.Resolution)
	timeout := opts.LaunchTimeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	timeoutMs := float64(timeout / time.Millisecond)

	args := defaultArgs(opts)
	exe := ResolveChromePath(opts)
	locale := fallback(opts.Locale, "en-US")

	launchOpts := playwright.BrowserTypeLaunchPersistentContextOptions{
		Headless:       playwright.Bool(opts.Headless),
		ExecutablePath: playwright.String(exe),
		Args:           args,
		Viewport: &playwright.Size{
			Width:  width,
			Height: height,
		},
		Locale:            playwright.String(locale),
		IgnoreHttpsErrors: playwright.Bool(true),
		AcceptDownloads:   playwright.Bool(true),
		BypassCSP:         playwright.Bool(true),
		Timeout:           playwright.Float(timeoutMs),
	}
	if opts.SlowMoMs > 0 {
		launchOpts.SlowMo = playwright.Float(float64(opts.SlowMoMs))
	}
	if opts.PermissionsMicrophone {
		launchOpts.Permissions = []string{"microphone", "camera"}
	} else {
		launchOpts.Permissions = []string{"camera"}
	}

	// TODO(user): mirror src/browser/browser.ts use of an empty user-data-dir
	// to get a fresh profile per session. playwright-go currently exposes
	// this via the first arg of LaunchPersistentContext.
	userDataDir := ""
	bctx, err := pw.Chromium.LaunchPersistentContext(userDataDir, launchOpts)
	if err != nil {
		_ = pw.Stop()
		return fmt.Errorf("browser: launch persistent context: %w", err)
	}

	d.pw = pw
	d.context = bctx
	log.Info("chromium launched")
	return nil
}

// Context returns the persistent context, or nil before Launch.
func (d *playwrightDriver) Context() domain.BrowserContext {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.context
}

// NewPage opens a new tab inside the persistent context.
//
// Wires the per-page diagnostic hook automatically ÃƒÂ¢Ã¢â€šÂ¬Ã¢â‚¬Â every domain.Page created
// via this method emits its console.log/pageerror/requestfailed records
// through the bot logger.
func (d *playwrightDriver) NewPage(ctx context.Context) (domain.Page, error) {
	d.mu.Lock()
	if d.context == nil {
		d.mu.Unlock()
		return nil, errors.New("browser: not launched")
	}
	c := d.context
	d.mu.Unlock()

	page, err := c.NewPage()
	if err != nil {
		return nil, fmt.Errorf("browser: new page: %w", err)
	}

	// Attach diagnostic logger. Caller does not need to manage teardown ÃƒÂ¢Ã¢â€šÂ¬Ã¢â‚¬Â
	// playwright-go cleans up handlers when the domain.Page closes.
	logger.PageHook(ctx, page)
	return page, nil
}

// Close tears the persistent context down then stops the Playwright runtime.
//
// Errors from the two close steps are joined so the caller sees both.
func (d *playwrightDriver) Close(_ context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	var errs []error
	if d.context != nil {
		if err := d.context.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close context: %w", err))
		}
		d.context = nil
	}
	if d.pw != nil {
		if err := d.pw.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("stop playwright: %w", err))
		}
		d.pw = nil
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func fallback(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
