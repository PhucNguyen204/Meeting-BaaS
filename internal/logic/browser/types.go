// Package browser wraps github.com/playwright-community/playwright-go to
// provide a small, opinionated API tailored to the bot's needs:
//
//   - One persistent Chromium context per bot session (matches src/browser/browser.ts).
//   - Predictable args list (launch_args.go) so behaviour is reproducible.
//   - Centralised diagnostic logging via PageHook (logger.PageHook).
//
// The interface in driver.go decouples the rest of the codebase from
// playwright-go directly so unit tests can stub it.
package browser

import (
	"github.com/playwright-community/playwright-go"
)

// Aliases — re-exported so callers don't need a direct import of
// playwright-go just to reference these types in a function signature.
type (
	Page           = playwright.Page
	BrowserContext = playwright.BrowserContext
	Browser        = playwright.Browser
	Locator        = playwright.Locator
)
