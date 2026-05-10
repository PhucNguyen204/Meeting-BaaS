package logger

import (
	"context"

	"go.uber.org/zap"
)

// ctxKey is unexported so external packages cannot accidentally collide.
type ctxKey struct{}

// loggerKey is the context value holding the current *zap.Logger.
var loggerKey = ctxKey{}

// nop is returned by FromContext when the context carries no logger.
// It is safe to call all zap methods on it; nothing is emitted.
var nop = zap.NewNop()

// IntoContext stores log on ctx so any downstream FromContext returns it.
//
// Usage:
//
//	ctx = logger.IntoContext(ctx, root)
//	doSomething(ctx)
func IntoContext(ctx context.Context, log *zap.Logger) context.Context {
	if log == nil {
		return ctx
	}
	return context.WithValue(ctx, loggerKey, log)
}

// FromContext returns the *zap.Logger stored on ctx, or a no-op logger if
// none is present. It never returns nil so callers can chain freely.
func FromContext(ctx context.Context) *zap.Logger {
	if ctx == nil {
		return nop
	}
	if v, ok := ctx.Value(loggerKey).(*zap.Logger); ok && v != nil {
		return v
	}
	return nop
}

// With returns ctx with a child logger that has fields attached.
//
// Useful at function entry to scope all subsequent logs:
//
//	ctx, log := logger.With(ctx, zap.String("page_id", id))
//	log.Debug("opened page")
func With(ctx context.Context, fields ...zap.Field) (context.Context, *zap.Logger) {
	child := FromContext(ctx).With(fields...)
	return IntoContext(ctx, child), child
}

// WithState attaches the current state machine state name.
// Mirrors the per-state logger used in [src/state-machine/states/base-state.ts].
func WithState(ctx context.Context, state string) context.Context {
	c, _ := With(ctx, zap.String("state", state))
	return c
}

// WithProvider attaches the meeting provider ("Meet" / "Teams" / "Zoom").
func WithProvider(ctx context.Context, provider string) context.Context {
	c, _ := With(ctx, zap.String("provider", provider))
	return c
}

// WithPageID attaches a Playwright page identifier so logs from concurrent
// pages can be disambiguated.
func WithPageID(ctx context.Context, pageID string) context.Context {
	c, _ := With(ctx, zap.String("page_id", pageID))
	return c
}

// WithRequestID attaches a request identifier (HTTP middleware injects this).
func WithRequestID(ctx context.Context, reqID string) context.Context {
	c, _ := With(ctx, zap.String("request_id", reqID))
	return c
}

// Convenience shortcuts. Prefer FromContext(ctx).Xxx for hot paths since
// these allocate one extra interface conversion.

// Debug logs at debug level using the context logger.
func Debug(ctx context.Context, msg string, fields ...zap.Field) {
	FromContext(ctx).Debug(msg, fields...)
}

// Info logs at info level using the context logger.
func Info(ctx context.Context, msg string, fields ...zap.Field) {
	FromContext(ctx).Info(msg, fields...)
}

// Warn logs at warn level using the context logger.
func Warn(ctx context.Context, msg string, fields ...zap.Field) {
	FromContext(ctx).Warn(msg, fields...)
}

// Error logs at error level using the context logger.
func Error(ctx context.Context, msg string, fields ...zap.Field) {
	FromContext(ctx).Error(msg, fields...)
}
