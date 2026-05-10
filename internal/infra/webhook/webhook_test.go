package webhook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewSender_Defaults(t *testing.T) {
	s := NewSender(nil, SenderOptions{})
	if s.maxFailures != 5 {
		t.Errorf("default maxFailures: %d", s.maxFailures)
	}
	if s.resetTimeout != 30*time.Second {
		t.Errorf("default resetTimeout: %v", s.resetTimeout)
	}
}

func TestSender_Send_Success(t *testing.T) {
	var received int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&received, 1)
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("failed to decode body: %v", err)
		}
		if body["event"] != "test" {
			t.Errorf("unexpected event: %v", body["event"])
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	s := NewSender(nil, SenderOptions{Timeout: 2 * time.Second})
	err := s.Send(context.Background(), ts.URL, map[string]string{"event": "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if atomic.LoadInt32(&received) != 1 {
		t.Errorf("expected 1 request, got %d", received)
	}
}

func TestSender_Send_EmptyURL(t *testing.T) {
	s := NewSender(nil, SenderOptions{})
	err := s.Send(context.Background(), "", map[string]string{"event": "test"})
	if err != nil {
		t.Errorf("empty URL should return nil: %v", err)
	}
}

func TestSender_Send_Retry_On_5xx(t *testing.T) {
	var attempts int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	s := NewSender(nil, SenderOptions{Timeout: 2 * time.Second})
	err := s.Send(context.Background(), ts.URL, map[string]string{"event": "retry"})
	if err != nil {
		t.Fatalf("expected success after retries: %v", err)
	}
	if atomic.LoadInt32(&attempts) != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestSender_Send_NoRetry_On_4xx(t *testing.T) {
	var attempts int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer ts.Close()

	s := NewSender(nil, SenderOptions{Timeout: 2 * time.Second})
	err := s.Send(context.Background(), ts.URL, map[string]string{"event": "bad"})
	if err == nil {
		t.Fatal("expected error for 400")
	}
	if atomic.LoadInt32(&attempts) != 1 {
		t.Errorf("expected 1 attempt (no retry for 4xx), got %d", attempts)
	}
}

func TestSender_CircuitBreaker(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	s := NewSender(nil, SenderOptions{
		Timeout:      time.Second,
		MaxFailures:  2,
		ResetTimeout: 100 * time.Millisecond,
	})

	// Exhaust circuit breaker (each Send does 3 attempts = 3 failures per call).
	_ = s.Send(context.Background(), ts.URL, "a")

	// Circuit should be open now.
	err := s.Send(context.Background(), ts.URL, "b")
	if err == nil {
		t.Fatal("expected circuit breaker error")
	}

	// Wait for reset.
	time.Sleep(150 * time.Millisecond)

	// Circuit should be closed now (half-open, next request determines).
	err = s.Send(context.Background(), ts.URL, "c")
	// Still fails (server 500) but circuit accepted the request.
	if err == nil {
		t.Fatal("expected error (server still 500)")
	}
}

func TestSender_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediately cancelled

	s := NewSender(nil, SenderOptions{})
	err := s.Send(ctx, "http://example.com/webhook", "test")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}
