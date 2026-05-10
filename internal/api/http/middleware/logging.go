package middleware

import (
	"net/http"
	"time"

	"go.uber.org/zap"
)

// RequestLogger writes one structured log line per HTTP request after the
// handler returns. Fields:
//
//	method, path, status, dur_ms, tenant_id, api_key_prefix, request_id
//
// Plays nicely with chi's RequestID middleware (read via r.Context()).
func RequestLogger(log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			lrw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(lrw, r)
			fields := []zap.Field{
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", lrw.status),
				zap.Int64("dur_ms", time.Since(start).Milliseconds()),
			}
			if t := TenantFromContext(r.Context()); t != "" {
				fields = append(fields, zap.String("tenant_id", t))
			}
			if p := APIKeyPrefixFromContext(r.Context()); p != "" {
				fields = append(fields, zap.String("api_key_prefix", p))
			}
			if log != nil {
				if lrw.status >= 500 {
					log.Error("http request", fields...)
				} else {
					log.Info("http request", fields...)
				}
			}
		})
	}
}

// statusWriter records the final status code for the logger.
type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (s *statusWriter) WriteHeader(status int) {
	if s.wrote {
		return
	}
	s.status = status
	s.wrote = true
	s.ResponseWriter.WriteHeader(status)
}

func (s *statusWriter) Write(p []byte) (int, error) {
	if !s.wrote {
		s.WriteHeader(http.StatusOK)
	}
	return s.ResponseWriter.Write(p)
}
