package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// TestAuth_MissingHeader returns 401.
func TestAuth_MissingHeader(t *testing.T) {
	handler := Auth(AuthDeps{})(okHandler)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v2/bots", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "UNAUTHORIZED") {
		t.Errorf("want envelope to mention UNAUTHORIZED, got %s", rr.Body.String())
	}
}

// TestAuth_RedisCacheHit verifies the middleware can resolve a token straight
// from the Redis cache without hitting Postgres.
func TestAuth_RedisCacheHit(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	const plaintext = "mb_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	// Pre-populate the cache with a value of the form id|tenant|prefix|rate.
	// Hash is sha256(plaintext) hex; the auth middleware reproduces this
	// at request time, so we mirror it here.
	cacheKey := "apikey:cache:" + sha256Hex(plaintext)
	mr.Set(cacheKey, "key-id|tenant-id|aaaaaaaa|600")

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if got := TenantFromContext(r.Context()); got != "tenant-id" {
			t.Errorf("expected tenant-id, got %q", got)
		}
		if got := APIKeyIDFromContext(r.Context()); got != "key-id" {
			t.Errorf("expected key-id, got %q", got)
		}
		if got := RateLimitFromContext(r.Context()); got != 600 {
			t.Errorf("expected 600/min, got %d", got)
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := Auth(AuthDeps{Redis: rdb})(next)
	req := httptest.NewRequest(http.MethodGet, "/v2/bots", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", rr.Code, rr.Body.String())
	}
	if !called {
		t.Fatal("downstream handler not invoked")
	}
}

// TestAuth_CustomHeader accepts the x-meeting-baas-api-key compat header.
func TestAuth_CustomHeader(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	const plaintext = "mb_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	mr.Set("apikey:cache:"+sha256Hex(plaintext), "k|t|bbbbbbbb|10")

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })
	handler := Auth(AuthDeps{Redis: rdb})(next)
	req := httptest.NewRequest(http.MethodGet, "/v2/bots", nil)
	req.Header.Set("x-meeting-baas-api-key", plaintext)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if !called {
		t.Fatalf("downstream not invoked; status=%d body=%s", rr.Code, rr.Body.String())
	}
}

// sha256Hex mirrors the helper inside resolveAPIKey so tests can pre-populate
// the Redis cache with the same key the middleware looks up.
func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
