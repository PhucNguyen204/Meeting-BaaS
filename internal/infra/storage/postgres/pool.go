// Package postgres provides a pgxpool-backed connection pool and the
// repository implementations for bots, bot_events, recordings, and
// webhook_deliveries.
//
// Phase 3 dependency. The api-server and controller bind on a shared *Pool;
// bot-worker may also use it for emitting events directly.
package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// Pool wraps pgxpool.Pool with a logger and convenience constructors.
type Pool struct {
	*pgxpool.Pool
	log *zap.Logger
}

// Options configures the connection pool.
type Options struct {
	// DSN is the libpq-style connection string.
	// e.g. postgres://user:pass@host:5432/db?sslmode=disable
	DSN string
	// MaxConns caps the pool. Defaults to 10.
	MaxConns int32
	// ConnectTimeout for the initial connect (default 10s).
	ConnectTimeout time.Duration
}

// New connects to Postgres using the given options.
//
// The returned Pool is safe for concurrent use; close with Close().
func New(ctx context.Context, log *zap.Logger, opts Options) (*Pool, error) {
	if log == nil {
		log = zap.NewNop()
	}
	if opts.DSN == "" {
		return nil, fmt.Errorf("postgres: empty DSN")
	}
	if opts.MaxConns <= 0 {
		opts.MaxConns = 10
	}
	if opts.ConnectTimeout <= 0 {
		opts.ConnectTimeout = 10 * time.Second
	}

	cfg, err := pgxpool.ParseConfig(opts.DSN)
	if err != nil {
		return nil, fmt.Errorf("postgres: parse dsn: %w", err)
	}
	cfg.MaxConns = opts.MaxConns
	cfg.ConnConfig.ConnectTimeout = opts.ConnectTimeout

	pingCtx, cancel := context.WithTimeout(ctx, opts.ConnectTimeout)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(pingCtx, cfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: new pool: %w", err)
	}
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}

	log.Info("postgres pool connected",
		zap.Int32("max_conns", cfg.MaxConns),
		zap.String("host", cfg.ConnConfig.Host),
		zap.String("database", cfg.ConnConfig.Database),
	)
	return &Pool{Pool: pool, log: log.Named("postgres")}, nil
}
