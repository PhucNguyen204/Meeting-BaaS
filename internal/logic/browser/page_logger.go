package browser

// page_logger.go is intentionally a thin re-export of the diagnostic hook
// in the logger package. It exists so callers in this package have a
// natural import path (`browser.AttachDiagnostics(...)`) without reaching
// into pkg/logger.
//
// Port reference: src/browser/page-logger.ts.

import (
	"context"

	"github.com/yourorg/meet-bot-go/internal/pkg/logger"
)

// AttachDiagnostics wires browser console / pageerror / requestfailed
// events through the context logger. Returns a teardown func.
//
// NewPage already calls this for every page it produces; use this helper
// only when wrapping pages produced outside the Driver (e.g. when iframing
// support is added later).
func AttachDiagnostics(ctx context.Context, page Page) (teardown func()) {
	return logger.PageHook(ctx, page)
}
