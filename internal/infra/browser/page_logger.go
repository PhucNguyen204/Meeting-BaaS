package browser

// page_logger.go is intentionally a thin re-export of the diagnostic hook
// in the logger package. It exists so callers in this package have a
// natural import path (`browser.AttachDiagnostics(...)`) without reaching
// into pkg/logger.
//
// Port reference: src/browser/page-logger.ts.

import (
	"context"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/domain"

	"github.com/PhucNguyen204/Meeting-BaaS/internal/pkg/logger"
)

// AttachDiagnostics wires browser console / pageerror / requestfailed
// events through the context logger. Returns a teardown func.
//
// NewPage already calls this for every domain.Page it produces; use this helper
// only when wrapping pages produced outside the domain.BrowserDriver (e.g. when iframing
// support is added later).
func AttachDiagnostics(ctx context.Context, page domain.Page) (teardown func()) {
	return logger.PageHook(ctx, page)
}
