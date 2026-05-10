package v2

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/PhucNguyen204/Meeting-BaaS/internal/api/http/middleware"
)

// Mount wires the v2 endpoints onto the given chi router. Caller is expected
// to apply Auth + RateLimit middleware on the parent group; this function
// only mounts Idempotency on the POST endpoints that actually accept it.
func Mount(r chi.Router, deps Deps, idem middlewareFn) {
	// Mutating endpoints: Idempotency-Key-aware.
	r.Group(func(r chi.Router) {
		if idem != nil {
			r.Use(idem)
		}
		r.Post("/bots", HandleCreateBot(deps))
		r.Post("/bots/scheduled", HandleScheduleBot(deps))
	})

	r.Get("/bots/{bot_id}", HandleGetBot(deps))
	r.Get("/bots/{bot_id}/status", HandleGetBotStatus(deps))
	r.Post("/bots/{bot_id}/leave-bot", HandleLeaveBot(deps))
	r.Post("/bots/{bot_id}/pause-recording", HandlePauseBot(deps))
	r.Post("/bots/{bot_id}/resume-recording", HandleResumeBot(deps))
	r.Post("/bots/{bot_id}/chat-messages", HandleSendChat(deps))
	r.Delete("/bots/{bot_id}/delete-data", HandleDeleteData(deps))

	r.Get("/usage", HandleUsage(deps))
	r.Get("/alerts", HandleAlerts(deps))
}

// middlewareFn is the standard func(http.Handler) http.Handler shape so we
// can take either chi-style middleware or our own.
type middlewareFn = func(http.Handler) http.Handler

// Verify that middleware.WithTenant compiles in this package by referencing
// it; otherwise the package would be a candidate for the unused-import lint.
var _ = middleware.WithTenant
