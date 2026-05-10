package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/PhucNguyen204/Meeting-BaaS/internal/api/http/respond"
)

// RateLimit enforces the per-API-key per-minute quota set on api_keys.
//
// Implementation: atomic INCR on rate:{api_key_id}:{minute_window} with a 60s
// TTL set on the first increment. If the counter exceeds the limit configured
// on the key (RateLimitFromContext), we reject with 429 and Retry-After.
//
// When no api key context is present (request did not flow through Auth),
// the middleware is a no-op so the chain remains usable for /healthz etc.
func RateLimit(rdb *redis.Client) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			limit := RateLimitFromContext(r.Context())
			apiKeyID := APIKeyIDFromContext(r.Context())
			if rdb == nil || limit <= 0 || apiKeyID == "" {
				next.ServeHTTP(w, r)
				return
			}

			window := time.Now().UTC().Unix() / 60
			key := fmt.Sprintf("rate:%s:%d", apiKeyID, window)

			ctx, cancel := context.WithTimeout(r.Context(), 200*time.Millisecond)
			defer cancel()

			cnt, err := rdb.Incr(ctx, key).Result()
			if err != nil {
				// Fail-open: rather than 500ing the whole API when Redis is
				// flaky, log and let it through. The tradeoff is brief
				// over-allowance during outages.
				next.ServeHTTP(w, r)
				return
			}
			if cnt == 1 {
				// First hit in this window. Pin a 65s TTL (5s slack so the
				// counter cannot live past two windows on a slow clock).
				_ = rdb.Expire(ctx, key, 65*time.Second).Err()
			}

			if cnt > int64(limit) {
				retryAfter := 60 - (time.Now().UTC().Unix() % 60)
				w.Header().Set("Retry-After", strconv.FormatInt(retryAfter, 10))
				w.Header().Set("X-RateLimit-Limit", strconv.FormatInt(int64(limit), 10))
				w.Header().Set("X-RateLimit-Remaining", "0")
				respond.Error(w, http.StatusTooManyRequests, respond.CodeRateLimited,
					fmt.Sprintf("rate limit exceeded (%d/min); retry in %ds", limit, retryAfter))
				return
			}

			w.Header().Set("X-RateLimit-Limit", strconv.FormatInt(int64(limit), 10))
			w.Header().Set("X-RateLimit-Remaining", strconv.FormatInt(int64(limit)-cnt, 10))
			next.ServeHTTP(w, r)
		})
	}
}
