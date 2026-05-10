package http

import "net/http"

// handleHealthz returns 200 OK with a tiny body. Kubernetes uses this for
// both liveness and readiness probes; the bot is "ready" as soon as the
// HTTP server is up â€” actual meeting state is reported via webhooks.
//
// Port reference: src/server.ts has no explicit /healthz, but the
// container Dockerfile uses curl on the port for liveness. This is the
// idiomatic Go equivalent.
func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}
