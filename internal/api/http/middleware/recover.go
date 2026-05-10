package middleware

import (
	"net/http"
	"runtime/debug"

	"go.uber.org/zap"

	"github.com/PhucNguyen204/Meeting-BaaS/internal/api/http/respond"
)

// Recover catches panics from downstream handlers, logs the stack, and emits
// a standard v2 INTERNAL error envelope so clients always see the same shape.
//
// Use this instead of chi/middleware.Recoverer for v2 routes because the chi
// one writes plain text "Internal Server Error" which clients can't parse.
func Recover(log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					if log != nil {
						log.Error("panic in HTTP handler",
							zap.Any("panic", rec),
							zap.String("path", r.URL.Path),
							zap.ByteString("stack", debug.Stack()),
						)
					}
					respond.Error(w, http.StatusInternalServerError, respond.CodeInternal, "internal server error")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
