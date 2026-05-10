// Package v2 implements the Meeting BaaS v2 REST surface served by api-server.
package v2

import (
	"net/http"

	"github.com/PhucNguyen204/Meeting-BaaS/internal/api/http/respond"
)

// Envelope and the ErrorBody live in the respond package so middleware can
// produce the same shape without creating an import cycle back into v2. We
// re-export the names and helpers here so v2 handlers stay terse.
type (
	Envelope  = respond.Envelope
	ErrorBody = respond.ErrorBody
)

const (
	CodeInvalidParameters   = respond.CodeInvalidParameters
	CodeUnauthorized        = respond.CodeUnauthorized
	CodeForbidden           = respond.CodeForbidden
	CodeNotFound            = respond.CodeNotFound
	CodeConflict            = respond.CodeConflict
	CodeIdempotencyConflict = respond.CodeIdempotencyConflict
	CodeRateLimited         = respond.CodeRateLimited
	CodeInsufficientTokens  = respond.CodeInsufficientTokens
	CodeInternal            = respond.CodeInternal
	CodeServiceUnavailable  = respond.CodeServiceUnavailable
)

// WriteOK / WriteCreated / WriteAccepted / WriteError / WriteJSON forward to
// the respond package so handlers can call them under the v2 alias.
func WriteOK(w http.ResponseWriter, data any)       { respond.OK(w, data) }
func WriteCreated(w http.ResponseWriter, data any)  { respond.Created(w, data) }
func WriteAccepted(w http.ResponseWriter, data any) { respond.Accepted(w, data) }

func WriteError(w http.ResponseWriter, status int, code, message string) {
	respond.Error(w, status, code, message)
}

func WriteErrorDetails(w http.ResponseWriter, status int, code, message string, details any) {
	respond.ErrorDetails(w, status, code, message, details)
}

func WriteJSON(w http.ResponseWriter, status int, body any) {
	respond.JSON(w, status, body)
}
