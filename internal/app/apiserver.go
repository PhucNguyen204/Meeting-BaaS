package app

import (
	"context"
	"errors"

	"go.uber.org/zap"
)

// APIServer is the runtime container for the api-server process.
//
// Phase 1 ships only a stub: the API server proper (queueing jobs onto
// Redis Streams, exposing CRUD over bots, dispatching webhooks) is Phase 3.
// This struct exists so cmd/api-server/main.go can compile and be slowly
// fleshed out.
type APIServer struct {
	Logger *zap.Logger
}

// NewAPIServer is the (stub) constructor. Returns ErrNotImplemented when
// invoked at runtime to make the placeholder loud rather than silent.
func NewAPIServer(log *zap.Logger) (*APIServer, error) {
	if log == nil {
		log = zap.NewNop()
	}
	return &APIServer{Logger: log}, nil
}

// ErrNotImplemented is returned by APIServer.Run while it remains a stub.
var ErrNotImplemented = errors.New("api-server not implemented in phase 1")

// Run logs and returns ErrNotImplemented. Replace in Phase 3.
func (a *APIServer) Run(_ context.Context) error {
	a.Logger.Warn("api-server is a Phase 1 stub")
	return ErrNotImplemented
}
