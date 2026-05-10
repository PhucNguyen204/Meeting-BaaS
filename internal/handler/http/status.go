package http

import (
	"encoding/json"
	"net/http"
	"time"
)

// StatusProvider returns runtime status info. Wired to the state machine.
type StatusProvider interface {
	CurrentState() string
	StartTime() int64
	IsPaused() bool
	EndReason() string
}

// statusResponse is the JSON response for GET /status.
type statusResponse struct {
	State     string `json:"state"`
	StartTime int64  `json:"start_time,omitempty"`
	Uptime    string `json:"uptime,omitempty"`
	IsPaused  bool   `json:"is_paused"`
	EndReason string `json:"end_reason,omitempty"`
}

// handleStatus returns the bot's current runtime status.
func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	if s.statusProvider == nil {
		writeJSON(w, http.StatusOK, statusResponse{State: "initializing"})
		return
	}

	resp := statusResponse{
		State:     s.statusProvider.CurrentState(),
		StartTime: s.statusProvider.StartTime(),
		IsPaused:  s.statusProvider.IsPaused(),
		EndReason: s.statusProvider.EndReason(),
	}
	if resp.StartTime > 0 {
		startTime := time.UnixMilli(resp.StartTime)
		resp.Uptime = time.Since(startTime).Truncate(time.Second).String()
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
