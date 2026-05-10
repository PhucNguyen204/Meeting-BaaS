package http

import (
	"encoding/json"
	"net/http"

	"go.uber.org/zap"
)

// stopRecordRequest is the request body shape.
//
// The TS server accepts an empty body but optionally honours
// `{ "reason": "..." }` for diagnostics.
type stopRecordRequest struct {
	Reason string `json:"reason,omitempty"`
}

// stopRecordResponse is the response body shape.
type stopRecordResponse struct {
	Status string `json:"status"`
}

// handleStopRecord requests graceful termination of the recording.
//
// Port reference: src/server.ts:73 (POST /stop_record handler).
//
// Behaviour:
//   - Decodes optional JSON body. Empty body is allowed.
//   - Calls Stopper.Stop(ctx, reason). Errors -> 500.
//   - Returns 200 with `{"status":"stopping"}` on success.
//
// The actual stop is asynchronous: the state machine transitions to
// Cleanup and the recorder finalises in the background. Operators must
// poll bot status via the upstream API server (or wait for the webhook).
func (s *Server) handleStopRecord(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var req stopRecordRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
			return
		}
	}
	reason := req.Reason
	if reason == "" {
		reason = "api_request"
	}

	if s.stopper == nil {
		s.log.Warn("stop_record: no stopper wired (Phase 1 stub)")
		writeJSON(w, http.StatusOK, stopRecordResponse{Status: "noop"})
		return
	}

	if err := s.stopper.Stop(r.Context(), reason); err != nil {
		s.log.Error("stop_record failed", zap.Error(err))
		http.Error(w, "stop failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, stopRecordResponse{Status: "stopping"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
