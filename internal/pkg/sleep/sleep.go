// Package sleep provides a context-aware sleep that returns ctx.Err() when
// the context is cancelled before d elapses. Mirrors src/utils/sleep.ts.
package sleep

import (
	"context"
	"time"
)

// For sleeps for d, returning early if ctx is cancelled.
//
// Port reference: src/utils/sleep.ts.
//
// Returns ctx.Err() if the context fired (Canceled / DeadlineExceeded);
// returns nil otherwise.
func For(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
