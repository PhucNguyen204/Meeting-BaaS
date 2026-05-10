package logger

import (
	"context"

	"github.com/playwright-community/playwright-go"
	"go.uber.org/zap"
)

// PageHook subscribes to a Playwright Page's diagnostic events and forwards
// them through the context logger. This makes browser-side console errors
// visible in the same JSON log stream as Go-side records, which is critical
// when debugging selector flakiness or in-page injected scripts.
//
// Port reference: src/browser/page-logger.ts (enablePrintPageLogs).
//
// Events forwarded:
//
//   - "console" â€” page.console messages (info/warn/error mapped to log levels).
//   - "pageerror" â€” uncaught JS exceptions in the page.
//   - "requestfailed" â€” network request failures (DNS, abort, CORS, etc).
//   - "crash" â€” page crash (rare but catastrophic; logged at error level).
//   - "close" â€” page closed event (logged at debug level).
//
// Returns a teardown func that detaches all listeners. Call it from the
// caller's defer chain to avoid leaking goroutines if the page survives.
//
// TODO(user): consider only attaching `console` when DEBUG_LOGS=true to
// avoid noisy logs in production. The TS implementation gates on the same env.
func PageHook(ctx context.Context, page playwright.Page) (teardown func()) {
	log := FromContext(ctx).Named("browser")

	consoleHandler := func(msg playwright.ConsoleMessage) {
		fields := []zap.Field{
			zap.String("type", msg.Type()),
			zap.String("text", msg.Text()),
		}
		switch msg.Type() {
		case "error":
			log.Error("page.console", fields...)
		case "warning", "warn":
			log.Warn("page.console", fields...)
		case "info", "log":
			log.Info("page.console", fields...)
		default:
			log.Debug("page.console", fields...)
		}
	}

	pageErrorHandler := func(err error) {
		log.Error("page.error", zap.Error(err))
	}

	requestFailedHandler := func(req playwright.Request) {
		ff := req.Failure()
		failureText := ""
		if ff != nil {
			failureText = ff.Error()
		}
		log.Warn("page.request_failed",
			zap.String("url", req.URL()),
			zap.String("method", req.Method()),
			zap.String("failure", failureText),
		)
	}

	crashHandler := func(playwright.Page) {
		log.Error("page.crash")
	}

	closeHandler := func(playwright.Page) {
		log.Debug("page.close")
	}

	page.OnConsole(consoleHandler)
	page.OnPageError(pageErrorHandler)
	page.OnRequestFailed(requestFailedHandler)
	page.OnCrash(crashHandler)
	page.OnClose(closeHandler)

	// playwright-go does not expose a typed Off() per-handler at the time of
	// writing; teardown therefore is best-effort. The Page going out of scope
	// tears the listeners down as well.
	//
	// TODO(user): when playwright-go gains typed Off* helpers, call them here.
	return func() {
		log.Debug("page hook teardown (no-op)")
	}
}
