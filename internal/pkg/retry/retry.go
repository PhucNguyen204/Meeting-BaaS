// Package retry provides exponential-backoff retry for synchronous
// operations. Used by the browser launcher and various network calls.
package retry

import (
	"context"
	"errors"
	"time"

	"github.com/yourorg/meet-bot-go/internal/pkg/sleep"
)

// Options tunes the retry behaviour. Zero values mean "use defaults".
type Options struct {
	// MaxAttempts is the total number of attempts (>=1). Zero defaults to 3.
	MaxAttempts int

	// InitialDelay before the second attempt. Zero defaults to 1s.
	InitialDelay time.Duration

	// MaxDelay caps the exponential backoff. Zero defaults to 30s.
	MaxDelay time.Duration

	// Multiplier is the backoff factor. Zero defaults to 2.0.
	Multiplier float64

	// IsRetryable optionally inspects an error to decide whether to retry.
	// Nil means "retry every error".
	IsRetryable func(error) bool
}

// Default returns sensible defaults: 3 attempts, 1s -> 2s -> 4s.
func Default() Options {
	return Options{
		MaxAttempts:  3,
		InitialDelay: time.Second,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
	}
}

// Do invokes fn, retrying on error per opts. Returns the last error from fn,
// or ctx.Err() if cancelled mid-backoff.
//
// Port reference: pattern from
// [src/state-machine/states/initialization-state.ts:68-136] (browser setup
// retry with progressive wait).
func Do(ctx context.Context, opts Options, fn func(ctx context.Context, attempt int) error) error {
	if opts.MaxAttempts <= 0 {
		opts.MaxAttempts = 3
	}
	if opts.InitialDelay <= 0 {
		opts.InitialDelay = time.Second
	}
	if opts.MaxDelay <= 0 {
		opts.MaxDelay = 30 * time.Second
	}
	if opts.Multiplier <= 0 {
		opts.Multiplier = 2.0
	}

	var lastErr error
	delay := opts.InitialDelay
	for attempt := 1; attempt <= opts.MaxAttempts; attempt++ {
		err := fn(ctx, attempt)
		if err == nil {
			return nil
		}
		lastErr = err

		if opts.IsRetryable != nil && !opts.IsRetryable(err) {
			return err
		}
		if attempt == opts.MaxAttempts {
			break
		}
		if sleepErr := sleep.For(ctx, delay); sleepErr != nil {
			return errors.Join(lastErr, sleepErr)
		}
		// Exponential backoff with cap.
		next := time.Duration(float64(delay) * opts.Multiplier)
		if next > opts.MaxDelay {
			next = opts.MaxDelay
		}
		delay = next
	}
	return lastErr
}
