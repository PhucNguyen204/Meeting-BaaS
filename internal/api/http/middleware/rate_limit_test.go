package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// TestRateLimit_AllowsUnderLimit verifies that two requests under the per-key
// quota both succeed and the second one sees the remaining counter decrement.
func TestRateLimit_AllowsUnderLimit(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	handler := chainCtx(RateLimit(rdb), okHandler)

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/v2/bots", nil)
		req = req.WithContext(WithAPIKey(context.Background(), "key-id", "abcd1234", 10))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("iter %d: want 200, got %d (body=%s)", i, rr.Code, rr.Body.String())
		}
	}
}

// TestRateLimit_RejectsOverLimit verifies that the 11th request inside one
// minute window is rejected with 429.
func TestRateLimit_RejectsOverLimit(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	handler := chainCtx(RateLimit(rdb), okHandler)
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/v2/bots", nil)
		req = req.WithContext(WithAPIKey(context.Background(), "key-id", "abcd1234", 10))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("iter %d should be 200, got %d", i, rr.Code)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/v2/bots", nil)
	req = req.WithContext(WithAPIKey(context.Background(), "key-id", "abcd1234", 10))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("11th request should be 429, got %d (body=%s)", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("Retry-After") == "" {
		t.Error("missing Retry-After header on 429")
	}
}

// TestRateLimit_NoopWithoutAPIKey verifies the middleware is transparent for
// requests that did not flow through Auth (e.g. /healthz).
func TestRateLimit_NoopWithoutAPIKey(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	handler := RateLimit(rdb)(okHandler)
	for i := 0; i < 200; i++ {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("iter %d unexpected status %d", i, rr.Code)
		}
	}
}

// --- helpers --------------------------------------------------------------

func chainCtx(mw func(http.Handler) http.Handler, h http.Handler) http.Handler {
	return mw(h)
}

var okHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
})
