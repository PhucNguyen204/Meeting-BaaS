package logger

import (
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// File sink: rotating per-bot log file at <logDir>/<botUUID>/bot.log.
//
// Mirrors the file logging behaviour from [src/utils/Logger.ts] which
// writes a single bot.log per invocation, then uploads it to S3 at
// session end (Phase 4).
//
// Rotation defaults are deliberately conservative â€” bot sessions usually
// stay within a single file but we cap to 100 MB per file just in case
// DEBUG_LOGS leaves something verbose running. Rotation parameters can be
// tuned later via [Config] without breaking callers.

const (
	defaultMaxSizeMB   = 100
	defaultMaxBackups  = 5
	defaultMaxAgeDays  = 14
	defaultPermDir     = 0o755
	defaultLogFileName = "bot.log"
)

// newFileSink returns a zap WriteSyncer that writes the per-bot rotating
// log file. Returns an error if the directory cannot be created.
//
// TODO(user): expose maxSize/maxBackups/maxAge through Config when the
// retention policy is decided.
func newFileSink(logDir, botUUID string) (zapcore.WriteSyncer, error) {
	if logDir == "" {
		return nil, fmt.Errorf("logger: empty log dir")
	}
	if botUUID == "" {
		return nil, fmt.Errorf("logger: empty bot uuid")
	}

	dir := filepath.Join(logDir, botUUID)
	if err := os.MkdirAll(dir, defaultPermDir); err != nil {
		return nil, fmt.Errorf("logger: mkdir %s: %w", dir, err)
	}

	lj := &lumberjack.Logger{
		Filename:   filepath.Join(dir, defaultLogFileName),
		MaxSize:    defaultMaxSizeMB,
		MaxBackups: defaultMaxBackups,
		MaxAge:     defaultMaxAgeDays,
		LocalTime:  false,
		Compress:   false,
	}
	return zapcore.AddSync(lj), nil
}

// LogFilePath returns where bot.log lives for a given UUID. Returned path
// may not yet exist if New was never called with file logging.
func LogFilePath(logDir, botUUID string) string {
	return filepath.Join(logDir, botUUID, defaultLogFileName)
}
