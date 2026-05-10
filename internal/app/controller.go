package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/PhucNguyen204/Meeting-BaaS/internal/infra/queue"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/pkg/logger"
)

// Controller is the runtime container for the controller process.
//
// Phase 3 / 6 deliverable. In Phase 3 it shells out to a local bot-worker
// binary via os/exec; in Phase 6 the same loop will instead create K8s Jobs.
//
// Lifecycle per job:
//
//  1. XREADGROUP a Job from the stream
//  2. spawn ./bin/bot-worker, piping Job.BotConfig to stdin
//  3. wait for the process; XACK on success
//  4. on error/timeout, leave unacked so it can be re-claimed via XPENDING
type Controller struct {
	Logger *zap.Logger

	rdb        *goredis.Client
	consumer   *queue.Consumer
	binaryPath string
	logDir     string
	maxParallel int

	// running tracks active subprocesses keyed by bot UUID so SIGTERM can
	// fan out cleanly. Currently informational; future PRs may expose it
	// via /admin endpoints.
	running   sync.Map // map[string]*exec.Cmd
	parallel  chan struct{}
}

// ControllerOptions tunes how NewController wires dependencies.
//
// Empty fields are filled from environment variables:
//
//	REDIS_ADDR        (default localhost:6379)
//	REDIS_PASSWORD
//	QUEUE_STREAM      (default queue.DefaultStream)
//	QUEUE_GROUP       (default queue.DefaultGroup)
//	QUEUE_CONSUMER    (default hostname or "controller-1")
//	BOT_WORKER_BINARY (default ./bin/bot-worker)
//	BOT_WORKER_LOG_DIR(default ./logs)
//	MAX_PARALLEL_BOTS (default 4)
type ControllerOptions struct {
	RedisAddr      string
	RedisPassword  string
	Stream         string
	Group          string
	Consumer       string
	BinaryPath     string
	LogDir         string
	MaxParallel    int
}

// NewController constructs the controller's dependency graph and ensures the
// consumer group exists (idempotent XGROUP CREATE … MKSTREAM).
func NewController(log *zap.Logger, opts ...ControllerOptions) (*Controller, error) {
	if log == nil {
		log = zap.NewNop()
	}
	var o ControllerOptions
	if len(opts) > 0 {
		o = opts[0]
	}
	o = applyControllerEnvDefaults(o)

	rdb := goredis.NewClient(&goredis.Options{
		Addr:     o.RedisAddr,
		Password: o.RedisPassword,
	})
	pingCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		return nil, fmt.Errorf("controller: redis ping: %w", err)
	}

	cons, err := queue.NewConsumer(context.Background(), log, rdb, queue.ConsumerOptions{
		Stream:   o.Stream,
		Group:    o.Group,
		Consumer: o.Consumer,
	})
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(o.LogDir, 0o755); err != nil {
		log.Warn("controller: cannot create log dir", zap.String("dir", o.LogDir), zap.Error(err))
	}

	c := &Controller{
		Logger:      log,
		rdb:         rdb,
		consumer:    cons,
		binaryPath:  o.BinaryPath,
		logDir:      o.LogDir,
		maxParallel: o.MaxParallel,
		parallel:    make(chan struct{}, o.MaxParallel),
	}
	return c, nil
}

// Run blocks until ctx is cancelled. It loops XREADGROUP → spawn → ACK with
// a small sleep between empty reads.
func (c *Controller) Run(ctx context.Context) error {
	ctx = logger.IntoContext(ctx, c.Logger)
	log := logger.FromContext(ctx)
	log.Info("controller starting",
		zap.String("binary", c.binaryPath),
		zap.Int("max_parallel", c.maxParallel),
	)

	defer func() { _ = c.rdb.Close() }()

	for {
		if err := ctx.Err(); err != nil {
			log.Info("controller stopping", zap.Error(err))
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case c.parallel <- struct{}{}:
		}

		job, streamID, err := c.consumer.ReadOne(ctx, 5*time.Second)
		if err != nil {
			<-c.parallel
			if errors.Is(err, context.Canceled) {
				return err
			}
			log.Warn("controller: read error, backing off", zap.Error(err))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(2 * time.Second):
			}
			continue
		}
		if job == nil {
			<-c.parallel
			continue
		}

		go func(job queue.Job, streamID string) {
			defer func() { <-c.parallel }()
			c.handleJob(ctx, job, streamID)
		}(*job, streamID)
	}
}

// handleJob spawns one bot-worker process for a job and ACKs on clean exit.
//
// On non-zero exit the message is left unacked. Operators can replay via
// XPENDING / XCLAIM. This avoids the simpler "ack-then-fail" anti-pattern.
func (c *Controller) handleJob(ctx context.Context, job queue.Job, streamID string) {
	log := c.Logger.With(
		zap.String("bot_id", job.BotID),
		zap.String("bot_uuid", job.BotUUID),
		zap.String("stream_id", streamID),
	)
	log.Info("controller: handling job")

	// Each bot writes its own log file alongside the bot-worker binary. The
	// binary already structures stdout/stderr via zap, but capturing them
	// here gives the controller a quick triage trail when a process dies.
	logPath := filepath.Join(c.logDir, fmt.Sprintf("bot-%s.log", safeFilename(job.BotUUID, job.BotID)))
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		log.Error("controller: cannot open bot log", zap.String("path", logPath), zap.Error(err))
		return
	}
	defer func() { _ = logFile.Close() }()

	cmd := exec.CommandContext(ctx, c.binaryPath)
	cmd.Stdin = bytes.NewReader(job.BotConfig)
	cmd.Stdout = io.MultiWriter(logFile, &prefixedWriter{log: log, level: zapcore.InfoLevel, tag: "stdout"})
	cmd.Stderr = io.MultiWriter(logFile, &prefixedWriter{log: log, level: zapcore.WarnLevel, tag: "stderr"})
	cmd.Env = append(os.Environ(),
		"BOT_UUID="+job.BotUUID,
		"BOT_ID="+job.BotID,
	)

	if err := cmd.Start(); err != nil {
		log.Error("controller: spawn failed", zap.Error(err))
		return
	}
	c.running.Store(job.BotUUID, cmd)
	defer c.running.Delete(job.BotUUID)

	waitErr := cmd.Wait()
	if waitErr != nil {
		log.Error("controller: bot-worker exited with error",
			zap.Int("pid", cmd.ProcessState.Pid()),
			zap.Int("exit_code", cmd.ProcessState.ExitCode()),
			zap.Error(waitErr),
		)
		// Intentionally leave streamID unacked so it can be re-claimed.
		return
	}

	log.Info("controller: bot-worker exited cleanly",
		zap.Int("pid", cmd.ProcessState.Pid()),
	)
	if err := c.consumer.Ack(ctx, streamID); err != nil {
		log.Error("controller: xack failed", zap.Error(err))
		return
	}
}

func applyControllerEnvDefaults(o ControllerOptions) ControllerOptions {
	if o.RedisAddr == "" {
		o.RedisAddr = envOr("REDIS_ADDR", "localhost:6379")
	}
	if o.RedisPassword == "" {
		o.RedisPassword = os.Getenv("REDIS_PASSWORD")
	}
	if o.Stream == "" {
		o.Stream = envOr("QUEUE_STREAM", queue.DefaultStream)
	}
	if o.Group == "" {
		o.Group = envOr("QUEUE_GROUP", queue.DefaultGroup)
	}
	if o.Consumer == "" {
		o.Consumer = os.Getenv("QUEUE_CONSUMER")
		if o.Consumer == "" {
			host, _ := os.Hostname()
			if host == "" {
				host = "controller-1"
			}
			o.Consumer = host
		}
	}
	if o.BinaryPath == "" {
		o.BinaryPath = envOr("BOT_WORKER_BINARY", defaultWorkerBinary())
	}
	if o.LogDir == "" {
		o.LogDir = envOr("BOT_WORKER_LOG_DIR", "./logs")
	}
	if o.MaxParallel <= 0 {
		if v, err := strconv.Atoi(os.Getenv("MAX_PARALLEL_BOTS")); err == nil && v > 0 {
			o.MaxParallel = v
		} else {
			o.MaxParallel = 4
		}
	}
	return o
}

// defaultWorkerBinary returns ./bin/bot-worker.exe on Windows, ./bin/bot-worker
// elsewhere. Helps the binary work in dev without cross-platform glue.
func defaultWorkerBinary() string {
	bin := "./bin/bot-worker"
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	return bin
}

// safeFilename derives a deterministic, safe-on-windows filename suffix.
func safeFilename(parts ...string) string {
	for _, p := range parts {
		if p != "" {
			return sanitize(p)
		}
	}
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}

func sanitize(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case ch >= 'a' && ch <= 'z',
			ch >= 'A' && ch <= 'Z',
			ch >= '0' && ch <= '9',
			ch == '-', ch == '_':
			out = append(out, ch)
		default:
			out = append(out, '_')
		}
	}
	return string(out)
}

// prefixedWriter forwards subprocess output line-by-line into the controller's
// structured logger so operators see job progress without tailing the log file.
type prefixedWriter struct {
	log   *zap.Logger
	level zapcore.Level
	tag   string
}

func (w *prefixedWriter) Write(p []byte) (int, error) {
	// Forward as a single info/warn entry per Write; we don't try to parse
	// embedded JSON (bot-worker's own zap log file already does that).
	if w.log != nil && len(bytes.TrimSpace(p)) > 0 {
		w.log.Log(w.level, "bot-worker output",
			zap.String("stream", w.tag),
			zap.ByteString("line", bytes.TrimRight(p, "\r\n")),
		)
	}
	return len(p), nil
}
