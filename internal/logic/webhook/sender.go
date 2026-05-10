// Package webhook delivers event payloads to external HTTP endpoints.
//
// Implements Circuit Breaker pattern for resilient delivery.
//
// Port reference: src/api.ts handleEndMeetingWithRetry().
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Sender delivers webhook payloads with retry and circuit breaker.
type Sender struct {
	log        *zap.Logger
	httpClient *http.Client

	// Circuit breaker state.
	mu            sync.Mutex
	failures      int
	lastFailure   time.Time
	circuitOpen   bool
	maxFailures   int
	resetTimeout  time.Duration
}

// SenderOptions configures the webhook sender.
type SenderOptions struct {
	Timeout        time.Duration // HTTP request timeout (default 10s)
	MaxRetries     int           // max delivery attempts (default 3)
	MaxFailures    int           // circuit breaker threshold (default 5)
	ResetTimeout   time.Duration // circuit breaker reset (default 30s)
}

// NewSender creates a webhook sender with circuit breaker.
func NewSender(log *zap.Logger, opts SenderOptions) *Sender {
	if log == nil {
		log = zap.NewNop()
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Second
	}
	if opts.MaxRetries <= 0 {
		opts.MaxRetries = 3
	}
	if opts.MaxFailures <= 0 {
		opts.MaxFailures = 5
	}
	if opts.ResetTimeout <= 0 {
		opts.ResetTimeout = 30 * time.Second
	}

	return &Sender{
		log:          log.Named("webhook"),
		httpClient:   &http.Client{Timeout: opts.Timeout},
		maxFailures:  opts.MaxFailures,
		resetTimeout: opts.ResetTimeout,
	}
}

// Send delivers a payload to the given URL with retry.
func (s *Sender) Send(ctx context.Context, url string, payload any) error {
	if url == "" {
		return nil // no webhook configured
	}

	if s.isCircuitOpen() {
		s.log.Warn("circuit breaker open, skipping webhook", zap.String("url", url))
		return fmt.Errorf("webhook: circuit breaker open")
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("webhook: marshal: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("webhook: build request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := s.httpClient.Do(req)
		if err != nil {
			lastErr = err
			s.recordFailure()
			s.log.Warn("webhook delivery failed",
				zap.Int("attempt", attempt+1),
				zap.Error(err),
			)
			time.Sleep(time.Duration(attempt+1) * time.Second)
			continue
		}
		_ = resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			s.recordSuccess()
			s.log.Info("webhook delivered",
				zap.String("url", url),
				zap.Int("status", resp.StatusCode),
			)
			return nil
		}

		lastErr = fmt.Errorf("webhook: HTTP %d", resp.StatusCode)
		if resp.StatusCode >= 500 {
			s.recordFailure()
			time.Sleep(time.Duration(attempt+1) * time.Second)
			continue
		}
		// 4xx errors are not retried.
		return lastErr
	}

	return fmt.Errorf("webhook: all retries exhausted: %w", lastErr)
}

func (s *Sender) isCircuitOpen() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.circuitOpen {
		return false
	}
	// Check if reset timeout has passed.
	if time.Since(s.lastFailure) >= s.resetTimeout {
		s.circuitOpen = false
		s.failures = 0
		s.log.Info("circuit breaker reset")
		return false
	}
	return true
}

func (s *Sender) recordFailure() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures++
	s.lastFailure = time.Now()
	if s.failures >= s.maxFailures {
		s.circuitOpen = true
		s.log.Warn("circuit breaker opened", zap.Int("failures", s.failures))
	}
}

func (s *Sender) recordSuccess() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures = 0
	s.circuitOpen = false
}
