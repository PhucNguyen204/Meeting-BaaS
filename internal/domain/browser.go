package domain

import (
	"context"
	"time"

	"github.com/playwright-community/playwright-go"
)

// Aliases for external types to avoid leaking Playwright dependencies
// everywhere, while keeping the interface compatible.
type (
	Page           = playwright.Page
	BrowserContext = playwright.BrowserContext
	Locator        = playwright.Locator
)

// BrowserLaunchOptions configures the persistent Chromium context.
type BrowserLaunchOptions struct {
	ChromePath            string
	Headless              bool
	Resolution            string
	SlowMoMs              int
	LaunchTimeout         time.Duration
	Locale                string
	PermissionsMicrophone bool
}

// BrowserDriver abstracts a Playwright runtime + persistent context.
type BrowserDriver interface {
	Launch(ctx context.Context, opts BrowserLaunchOptions) error
	Context() BrowserContext
	NewPage(ctx context.Context) (Page, error)
	Close(ctx context.Context) error
}
