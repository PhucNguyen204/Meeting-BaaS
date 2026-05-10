package http

import (
	"encoding/json"
	"net/http"

	"github.com/yourorg/meet-bot-go/internal/pkg/version"
)

// handleVersion returns the build metadata as JSON.
//
// Port reference: src/server.ts /version handler — same shape, different
// language.
func (s *Server) handleVersion(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(version.Get())
}
