package http

import (
	"encoding/json"
	"net/http"

	"github.com/PhucNguyen204/Meeting-BaaS/internal/pkg/version"
)

// handleVersion returns the build metadata as JSON.
//
// Port reference: src/server.ts /version handler Ã¢â‚¬â€ same shape, different
// language.
func (s *Server) handleVersion(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(version.Get())
}
