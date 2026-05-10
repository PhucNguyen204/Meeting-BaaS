package http

import (
	"context"
	"net/http"

	"go.uber.org/zap"
)

// PauseResumer controls recording pause/resume. Wired to the state machine.
type PauseResumer interface {
	Pause(ctx context.Context) error
	Resume(ctx context.Context) error
}

// handlePause pauses the recording.
func (s *Server) handlePause(w http.ResponseWriter, r *http.Request) {
	if s.pauseResumer == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "pause not available",
		})
		return
	}

	if err := s.pauseResumer.Pause(r.Context()); err != nil {
		s.log.Error("pause failed", zap.Error(err))
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "paused"})
}

// handleResume resumes the recording.
func (s *Server) handleResume(w http.ResponseWriter, r *http.Request) {
	if s.pauseResumer == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "resume not available",
		})
		return
	}

	if err := s.pauseResumer.Resume(r.Context()); err != nil {
		s.log.Error("resume failed", zap.Error(err))
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "resumed"})
}
