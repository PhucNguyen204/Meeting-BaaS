// Package logger wraps zap to provide a structured logger that:
//
//   - Always emits JSON to stdout (one record per line) so K8s/Loki picks it up.
//   - Optionally tees a copy to a per-bot rotating file under $LOG_DIR/<bot_uuid>/bot.log.
//   - Carries a context-scoped logger so we can attach state machine fields
//     (state, provider, page_id, request_id) without threading a *zap.Logger
//     through every function signature.
//   - Hooks Playwright Page events (console, pageerror, requestfailed) so the
//     same structured log stream contains browser-side diagnostics. See
//     [page_hook.go] and TS reference [src/browser/page-logger.ts].
//   - Masks well-known secret keys before logging arbitrary maps. See [mask.go]
//     and TS reference [src/main.ts:217-224].
//
// Port reference (high-level): src/utils/Logger.ts.
//
// Usage:
//
//	cfg := logger.ConfigFromEnv()
//	cfg.BotUUID = botUUID
//	root, err := logger.New(cfg)
//	if err != nil { /* ... */ }
//	defer root.Sync()
//
//	ctx := logger.IntoContext(context.Background(), root)
//	ctx = logger.WithState(ctx, "WaitingRoom")
//	logger.FromContext(ctx).Info("entering waiting room")
package logger

import (
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Config controls how the root logger is built.
//
// All fields are optional. ConfigFromEnv populates this struct from the
// process environment (LOG_LEVEL, LOG_DIR, DEBUG_LOGS, BOT_UUID, ...).
type Config struct {
	// Level is the minimum level to emit. Empty defaults to "info".
	Level string

	// Format is "json" (default) or "console".
	Format string

	// LogDir is the base directory for per-bot log files.
	// Empty disables file logging entirely.
	LogDir string

	// BotUUID is appended as a permanent field on every record and used
	// to compute the per-bot log file path: <LogDir>/<BotUUID>/bot.log.
	BotUUID string

	// Service is the binary name (e.g. "bot-worker", "api-server").
	Service string

	// Version is the build version, typically injected via -ldflags.
	Version string

	// EnableCaller adds file:line of the caller. Defaults to true.
	EnableCaller bool

	// EnableStacktrace attaches stacktrace from Error level upwards.
	EnableStacktrace bool
}

// ConfigFromEnv reads LOG_LEVEL, DEBUG_LOGS, LOG_DIR and friends from os.Getenv.
//
// LOG_LEVEL takes precedence over DEBUG_LOGS. DEBUG_LOGS=true is a shortcut
// for LOG_LEVEL=debug to mirror the TS [src/main.ts:42] convention.
//
// TODO(user): wire additional env keys (LOG_FORMAT, LOG_NO_FILE, ...) as needed.
func ConfigFromEnv() Config {
	level := os.Getenv("LOG_LEVEL")
	if level == "" && strings.EqualFold(os.Getenv("DEBUG_LOGS"), "true") {
		level = "debug"
	}
	return Config{
		Level:            level,
		Format:           os.Getenv("LOG_FORMAT"),
		LogDir:           os.Getenv("LOG_DIR"),
		BotUUID:          os.Getenv("BOT_UUID"),
		Service:          os.Getenv("SERVICE_NAME"),
		Version:          os.Getenv("APP_VERSION"),
		EnableCaller:     true,
		EnableStacktrace: false,
	}
}

// New builds the root *zap.Logger from cfg.
//
// Two cores are wired:
//  1. JSON encoder writing to os.Stdout at cfg.Level.
//  2. JSON encoder writing to the rotating file sink (file_sink.go) when
//     cfg.LogDir and cfg.BotUUID are both set.
//
// The returned logger has cfg.Service, cfg.Version, cfg.BotUUID baked in so
// every record carries them automatically.
//
// TODO(user): tweak EncoderConfig (timestamp format, level naming) here once
// the team agrees on a log schema. Default mirrors zap.NewProduction.
func New(cfg Config) (*zap.Logger, error) {
	level, err := parseLevel(cfg.Level)
	if err != nil {
		return nil, fmt.Errorf("logger: parse level: %w", err)
	}

	encCfg := zap.NewProductionEncoderConfig()
	encCfg.TimeKey = "ts"
	encCfg.LevelKey = "lvl"
	encCfg.MessageKey = "msg"
	encCfg.CallerKey = "caller"
	encCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	encCfg.EncodeLevel = zapcore.LowercaseLevelEncoder

	var enc zapcore.Encoder
	switch strings.ToLower(cfg.Format) {
	case "console":
		enc = zapcore.NewConsoleEncoder(encCfg)
	default:
		enc = zapcore.NewJSONEncoder(encCfg)
	}

	cores := []zapcore.Core{
		zapcore.NewCore(enc, zapcore.Lock(os.Stdout), level),
	}

	// Optional file sink. Errors here are NOT fatal â€” the stdout core is
	// always available so the bot can keep running with degraded logging.
	if cfg.LogDir != "" && cfg.BotUUID != "" {
		ws, err := newFileSink(cfg.LogDir, cfg.BotUUID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "logger: file sink disabled: %v\n", err)
		} else {
			cores = append(cores, zapcore.NewCore(enc, ws, level))
		}
	}

	opts := []zap.Option{}
	if cfg.EnableCaller {
		opts = append(opts, zap.AddCaller(), zap.AddCallerSkip(0))
	}
	if cfg.EnableStacktrace {
		opts = append(opts, zap.AddStacktrace(zapcore.ErrorLevel))
	}

	root := zap.New(zapcore.NewTee(cores...), opts...)

	// Permanent fields. Skip empty so zero-value Config still produces clean records.
	fields := []zap.Field{}
	if cfg.Service != "" {
		fields = append(fields, zap.String("service", cfg.Service))
	}
	if cfg.Version != "" {
		fields = append(fields, zap.String("version", cfg.Version))
	}
	if cfg.BotUUID != "" {
		fields = append(fields, zap.String("bot_uuid", cfg.BotUUID))
	}
	if len(fields) > 0 {
		root = root.With(fields...)
	}
	return root, nil
}

// parseLevel maps a string to zap level. Empty defaults to InfoLevel.
//
// Accepts: "trace" (=> debug), "debug", "info", "warn"/"warning", "error",
// "fatal". Unknown values return an error so configuration mistakes surface
// immediately at startup.
func parseLevel(s string) (zapcore.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "info":
		return zapcore.InfoLevel, nil
	case "trace", "debug":
		return zapcore.DebugLevel, nil
	case "warn", "warning":
		return zapcore.WarnLevel, nil
	case "error":
		return zapcore.ErrorLevel, nil
	case "fatal":
		return zapcore.FatalLevel, nil
	default:
		return zapcore.InfoLevel, fmt.Errorf("unknown log level %q", s)
	}
}

// MustNew panics if New fails. Convenient for cmd/*/main.go where there is
// nothing better to do with a logger error than abort.
func MustNew(cfg Config) *zap.Logger {
	l, err := New(cfg)
	if err != nil {
		panic(err)
	}
	return l
}
