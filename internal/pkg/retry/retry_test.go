package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDo_Success(t *testing.T) {
	err := Do(context.Background(), Default(), func(_ context.Context, _ int) error { return nil })
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestDo_RetrySuccess(t *testing.T) {
	n := 0
	err := Do(context.Background(), Options{MaxAttempts: 3, InitialDelay: time.Millisecond}, func(_ context.Context, _ int) error {
		n++
		if n < 3 { return errors.New("fail") }
		return nil
	})
	if err != nil { t.Fatalf("unexpected: %v", err) }
	if n != 3 { t.Errorf("got %d calls", n) }
}

func TestDo_Exhausted(t *testing.T) {
	err := Do(context.Background(), Options{MaxAttempts: 2, InitialDelay: time.Millisecond}, func(_ context.Context, _ int) error {
		return errors.New("fail")
	})
	if err == nil { t.Fatal("expected error") }
}

func TestDo_NotRetryable(t *testing.T) {
	perm := errors.New("perm")
	n := 0
	err := Do(context.Background(), Options{MaxAttempts: 5, InitialDelay: time.Millisecond, IsRetryable: func(e error) bool { return !errors.Is(e, perm) }}, func(_ context.Context, _ int) error {
		n++; return perm
	})
	if n != 1 { t.Errorf("got %d", n) }
	if !errors.Is(err, perm) { t.Error("wrong error") }
}

func TestDo_Cancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	n := 0
	_ = Do(ctx, Options{MaxAttempts: 10, InitialDelay: 50 * time.Millisecond}, func(_ context.Context, _ int) error {
		n++; if n == 2 { cancel() }; return errors.New("f")
	})
}

func TestDefault(t *testing.T) {
	d := Default()
	if d.MaxAttempts != 3 { t.Error("bad MaxAttempts") }
	if d.Multiplier != 2.0 { t.Error("bad Multiplier") }
}

func TestDo_BackoffCap(t *testing.T) {
	start := time.Now()
	_ = Do(context.Background(), Options{MaxAttempts: 3, InitialDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond, Multiplier: 100}, func(_ context.Context, _ int) error { return errors.New("f") })
	if time.Since(start) > 500*time.Millisecond { t.Error("too slow") }
}
