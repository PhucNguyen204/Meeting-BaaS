package sleep

import (
	"context"
	"testing"
	"time"
)

func TestFor_Normal(t *testing.T) {
	start := time.Now()
	err := For(context.Background(), 50*time.Millisecond)
	elapsed := time.Since(start)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if elapsed < 40*time.Millisecond {
		t.Errorf("sleep too short: %v", elapsed)
	}
}

func TestFor_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	err := For(ctx, 5*time.Second)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
	if elapsed > 100*time.Millisecond {
		t.Errorf("should return immediately on cancelled context: %v", elapsed)
	}
}

func TestFor_ContextDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := For(ctx, 5*time.Second)
	if err == nil {
		t.Fatal("expected error for deadline")
	}
}

func TestFor_ZeroDuration(t *testing.T) {
	err := For(context.Background(), 0)
	if err != nil {
		t.Errorf("zero duration should return nil: %v", err)
	}
}

func TestFor_NegativeDuration(t *testing.T) {
	err := For(context.Background(), -time.Second)
	if err != nil {
		t.Errorf("negative duration should return nil: %v", err)
	}
}

func TestFor_NegativeDuration_CancelledCtx(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := For(ctx, -time.Second)
	if err == nil {
		t.Fatal("expected error for cancelled ctx with negative duration")
	}
}
