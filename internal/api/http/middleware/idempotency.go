package middleware

import (
	"bytes"
	"crypto/sha256"
	"io"
	"net/http"

	"github.com/PhucNguyen204/Meeting-BaaS/internal/api/http/respond"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/infra/storage/postgres"
)

// Idempotency reads the Idempotency-Key header (only on POST requests) and
// consults the per-tenant idempotency_keys table. On replay it short-circuits
// with the cached response so the handler never runs twice.
//
// Tenant scoping is taken from TenantFromContext (set by Auth); if that is
// empty the middleware is a no-op so it stays usable in unauthenticated test
// scaffolds.
//
// Semantics:
//   - missing header        -> pass through; no caching.
//   - first request         -> run handler, capture response, persist body.
//   - replay, hash matches  -> return cached status + body.
//   - replay, hash differs  -> 409 IDEMPOTENCY_CONFLICT.
func Idempotency(repo *postgres.IdempotencyRepo) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost && r.Method != http.MethodDelete && r.Method != http.MethodPut {
				next.ServeHTTP(w, r)
				return
			}
			key := r.Header.Get("Idempotency-Key")
			tenantID := TenantFromContext(r.Context())
			if repo == nil || key == "" || tenantID == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Read + re-attach the body so we can hash without consuming it.
			body, err := io.ReadAll(r.Body)
			if err != nil {
				respond.Error(w, http.StatusBadRequest, respond.CodeInvalidParameters,
					"could not read request body")
				return
			}
			_ = r.Body.Close()
			r.Body = io.NopCloser(bytes.NewReader(body))

			hash := sha256.Sum256(body)
			hit, err := repo.BeginRequest(r.Context(), tenantID, key, hash[:])
			if err != nil {
				respond.Error(w, http.StatusInternalServerError, respond.CodeInternal,
					"idempotency: "+err.Error())
				return
			}

			if !hit.IsFirst {
				if !hit.HashMatch {
					respond.Error(w, http.StatusConflict, respond.CodeIdempotencyConflict,
						"Idempotency-Key reused with a different request body")
					return
				}
				if hit.Status == 0 {
					// Previous request still in flight; the spec recommends
					// 409 with a hint to retry shortly.
					respond.Error(w, http.StatusConflict, respond.CodeConflict,
						"a request with this Idempotency-Key is still being processed")
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Idempotent-Replayed", "true")
				w.WriteHeader(hit.Status)
				_, _ = w.Write(hit.Body)
				return
			}

			// First time we see this key. Wrap the writer so we can capture
			// what the handler writes.
			rec := &capturingWriter{ResponseWriter: w, status: http.StatusOK, body: &bytes.Buffer{}}
			next.ServeHTTP(rec, r)

			// Store async-style: failure to persist should not affect the
			// client-visible response (it's already written).
			_ = repo.CompleteRequest(r.Context(), tenantID, key, rec.status, rec.body.Bytes())
		})
	}
}

// capturingWriter is a tiny ResponseWriter shim that mirrors writes both to
// the underlying writer and to an in-memory buffer so we can persist the body
// for the idempotency cache.
type capturingWriter struct {
	http.ResponseWriter
	status int
	body   *bytes.Buffer
	wrote  bool
}

func (c *capturingWriter) WriteHeader(status int) {
	c.status = status
	c.wrote = true
	c.ResponseWriter.WriteHeader(status)
}

func (c *capturingWriter) Write(p []byte) (int, error) {
	if !c.wrote {
		c.WriteHeader(http.StatusOK)
	}
	c.body.Write(p)
	return c.ResponseWriter.Write(p)
}
