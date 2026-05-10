package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/PhucNguyen204/Meeting-BaaS/internal/api/http/respond"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/infra/storage/postgres"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/pkg/logger"
)

// apiKeyCacheTTL is the lifetime of the Redis-cached lookup row. Five minutes
// strikes a balance between fast hot-path resolution (avoid Postgres) and
// quickly honouring revokes / scope changes.
const apiKeyCacheTTL = 5 * time.Minute

// AuthDeps groups the dependencies the Auth middleware needs.
type AuthDeps struct {
	APIKeys *postgres.APIKeyRepo
	Redis   *redis.Client
	Logger  *zap.Logger
}

// Auth resolves the inbound token into a tenant + api key context entry.
//
// Accepted token transports (matching Meeting BaaS v2):
//
//	Authorization: Bearer <token>
//	x-meeting-baas-api-key: <token>
//
// On miss / revoked / expired, the middleware writes a 401 envelope and
// stops the chain. On hit, it injects tenant + api key id + per-key rate
// limit so RateLimit / handlers can read them via the context helpers.
func Auth(deps AuthDeps) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractToken(r)
			if token == "" {
				respond.Error(w, http.StatusUnauthorized, respond.CodeUnauthorized,
					"missing API key (Authorization: Bearer ... or x-meeting-baas-api-key)")
				return
			}

			key, err := resolveAPIKey(r.Context(), deps, token)
			if err != nil || key == nil {
				respond.Error(w, http.StatusUnauthorized, respond.CodeUnauthorized, "invalid API key")
				return
			}

			ctx := WithTenant(r.Context(), key.TenantID)
			ctx = WithAPIKey(ctx, key.ID, key.KeyPrefix, key.RateLimitPerMin)

			// Best-effort last-used bump. Run in a goroutine so we don't block
			// the request; cap with a short timeout. Skip entirely when no
			// APIKeys repo is configured (e.g. unit tests using the Redis
			// cache only).
			if deps.APIKeys != nil {
				go func(id string) {
					bg, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					defer cancel()
					if err := deps.APIKeys.MarkUsed(bg, id); err != nil && deps.Logger != nil {
						deps.Logger.Debug("auth: mark used failed", zap.String("api_key_id", id), zap.Error(err))
					}
				}(key.ID)
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); h != "" {
		if strings.HasPrefix(strings.ToLower(h), "bearer ") {
			return strings.TrimSpace(h[len("Bearer "):])
		}
	}
	if h := r.Header.Get("x-meeting-baas-api-key"); h != "" {
		return strings.TrimSpace(h)
	}
	return ""
}

// resolveAPIKey checks the Redis cache first, then falls back to Postgres.
// On Postgres miss returns (nil, ErrNotFound) so the caller can render a
// generic 401 without leaking which case applied.
func resolveAPIKey(ctx context.Context, deps AuthDeps, plaintext string) (*postgres.APIKeyRow, error) {
	hash := sha256.Sum256([]byte(plaintext))
	cacheKey := "apikey:cache:" + hex.EncodeToString(hash[:])

	if deps.Redis != nil {
		if val, err := deps.Redis.Get(ctx, cacheKey).Result(); err == nil && val != "" {
			parts := strings.SplitN(val, "|", 4)
			if len(parts) == 4 {
				var rate int32
				if r, err := parseInt32(parts[3]); err == nil {
					rate = r
				}
				return &postgres.APIKeyRow{
					ID:              parts[0],
					TenantID:        parts[1],
					KeyPrefix:       parts[2],
					RateLimitPerMin: rate,
				}, nil
			}
		}
	}

	if deps.APIKeys == nil {
		return nil, errors.New("auth: no APIKeys repo configured")
	}
	row, err := deps.APIKeys.LookupByPlaintext(ctx, plaintext)
	if err != nil {
		return nil, err
	}
	if deps.Redis != nil {
		// Cache the four hot-path fields. Pipe-delimited rather than JSON to
		// keep parse cost trivial.
		val := strings.Join([]string{row.ID, row.TenantID, row.KeyPrefix, formatInt32(row.RateLimitPerMin)}, "|")
		_ = deps.Redis.Set(ctx, cacheKey, val, apiKeyCacheTTL).Err()
	}
	return row, nil
}

// parseInt32 / formatInt32 are tiny helpers to avoid importing strconv in
// just two spots; cost is negligible.
func parseInt32(s string) (int32, error) {
	var n int32
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, errors.New("not a number")
		}
		n = n*10 + int32(c-'0')
	}
	return n, nil
}

func formatInt32(n int32) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// LoggerFromAuth is a convenience wrapper that decorates the request-scoped
// logger with tenant + api key fields. Handlers should call this once at the
// top of their function so subsequent log lines carry the auth context.
func LoggerFromAuth(ctx context.Context) *zap.Logger {
	log := logger.FromContext(ctx)
	if t := TenantFromContext(ctx); t != "" {
		log = log.With(zap.String("tenant_id", t))
	}
	if p := APIKeyPrefixFromContext(ctx); p != "" {
		log = log.With(zap.String("api_key_prefix", p))
	}
	return log
}
