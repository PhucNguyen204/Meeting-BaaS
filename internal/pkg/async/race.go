// Package async provides small concurrency helpers tailored to the bot
// use cases (timeout-or-result, abort polling, callback-once).
//
// These map to TS patterns like Promise.race(...) used throughout
// [src/state-machine/states/*] and [src/meeting/meet.ts].
package async

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ErrTimeout is returned by RaceTimeout when the timeout fires before fn returns.
var ErrTimeout = errors.New("timeout")

// RaceTimeout runs fn and returns its result, or ErrTimeout if d elapses first.
//
// fn receives a derived context that is cancelled when the timeout fires, so
// well-behaved fn implementations can clean up promptly. The original ctx is
// also honoured: cancellation propagates to fn.
//
// Port reference: Promise.race patterns from
// [src/state-machine/states/initialization-state.ts:84-103] and
// [src/state-machine/states/recording-state.ts:223-232].
//
// TODO(user): replace `interface{}` with generics ([T any]) once the rest of
// the codebase moves to Go 1.18+ patterns. Keeping interface{} here for now
// to keep the helper trivially compatible with adapters that return *Page,
// bool, or any other type.
func RaceTimeout(ctx context.Context, d time.Duration, fn func(context.Context) (any, error)) (any, error) {
	if fn == nil {
		return nil, errors.New("async: nil fn")
	}

	derived, cancel := context.WithTimeout(ctx, d)
	defer cancel()

	type result struct {
		v   any
		err error
	}
	ch := make(chan result, 1)
	go func() {
		v, err := fn(derived)
		ch <- result{v: v, err: err}
	}()

	select {
	case r := <-ch:
		return r.v, r.err
	case <-derived.Done():
		// Distinguish timeout vs upstream cancellation.
		if errors.Is(derived.Err(), context.DeadlineExceeded) {
			return nil, ErrTimeout
		}
		return nil, derived.Err()
	}
}

// Once returns a func that calls fn at most once. Concurrent callers are
// serialized; only the first one runs fn.
//
// Useful for join-success callbacks where the underlying observer might
// fire multiple times but the state machine only wants to react once.
func Once(fn func()) func() {
	var o sync.Once
	return func() { o.Do(fn) }
}

// Poll calls check every interval until it returns (true, nil), or ctx
// is cancelled, or check returns a non-nil error.
//
// Returns nil on (true, nil); returns ctx.Err() on cancellation; returns
// the first non-nil err from check.
//
// TODO(user): consider adaptive backoff when implementing the recording
// state's checkEndConditions loop in Phase 2.
func Poll(ctx context.Context, interval time.Duration, check func(context.Context) (bool, error)) error {
	if interval <= 0 {
		interval = 100 * time.Millisecond
	}
	t := time.NewTicker(interval)
	defer t.Stop()

	// Check immediately so a true-on-first-call short-circuits.
	if ok, err := check(ctx); err != nil || ok {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			ok, err := check(ctx)
			if err != nil {
				return err
			}
			if ok {
				return nil
			}
		}
	}
}
