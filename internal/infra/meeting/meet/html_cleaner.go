package meet

import (
	"context"
	"fmt"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/domain"
)

// HtmlCleaner removes UI chrome we don't want in the recording (banners,
// notifications, "You're presenting" overlays, etc).
//
// Port reference: src/meeting/meet/htmlCleaner.ts.
//
// Two phases:
//
//  1. Inject a MutationObserver-based JS payload via page.AddInitScript so
//     the cleaner runs even on subsequent SPA navigations within Meet.
//
//  2. Optionally call ApplyOnce(page) to force a synchronous cleanup pass
//     right before recording starts.
type HtmlCleaner struct{}

// NewHtmlCleaner returns a new cleaner.
func NewHtmlCleaner() *HtmlCleaner { return &HtmlCleaner{} }

// Inject registers the cleaner JS as an init script for the page.
//
// TODO(user): paste the JS from src/meeting/meet/htmlCleaner.ts into
// htmlCleanerJS below.
func (c *HtmlCleaner) Inject(_ context.Context, page domain.Page) error {
	if page == nil {
		return fmt.Errorf("htmlcleaner: nil page")
	}
	// TODO(user): page.AddInitScript(playwright.Script{Content: htmlCleanerJS})
	return nil
}

// ApplyOnce evaluates the cleaner once on the live document.
//
// TODO(user): port the inline cleanup function.
func (c *HtmlCleaner) ApplyOnce(_ context.Context, page domain.Page) error {
	if page == nil {
		return fmt.Errorf("htmlcleaner: nil page")
	}
	// TODO(user): _, err := page.Evaluate(htmlCleanerOneShotJS)
	return nil
}

// htmlCleanerJS is the MutationObserver-driven payload.
//
// TODO(user): paste from src/meeting/meet/htmlCleaner.ts.
const htmlCleanerJS = `// TODO(user): port from src/meeting/meet/htmlCleaner.ts`

// htmlCleanerOneShotJS performs a synchronous cleanup pass.
//
// TODO(user): paste from src/meeting/meet/htmlCleaner.ts (the function
// invoked at recording start, separate from the MutationObserver).
const htmlCleanerOneShotJS = `// TODO(user): port from src/meeting/meet/htmlCleaner.ts`
