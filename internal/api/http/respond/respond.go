// Package respond defines the v2 response envelope and the standard error
// codes. Both the v2 handler package and the middleware package depend on it
// so they can render uniform responses without depending on each other (which
// would create an import cycle).
package respond

import (
	"encoding/json"
	"net/http"
)

// Envelope is the top-level JSON shape every v2 response uses.
type Envelope struct {
	Success bool       `json:"success"`
	Data    any        `json:"data,omitempty"`
	Error   *ErrorBody `json:"error,omitempty"`
}

// ErrorBody is the v2 standardised error payload.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// Public error codes. Mirror the Meeting BaaS v2 reference.
const (
	CodeInvalidParameters   = "INVALID_PARAMETERS"
	CodeUnauthorized        = "UNAUTHORIZED"
	CodeForbidden           = "FORBIDDEN"
	CodeNotFound            = "NOT_FOUND"
	CodeConflict            = "CONFLICT"
	CodeIdempotencyConflict = "IDEMPOTENCY_CONFLICT"
	CodeRateLimited         = "RATE_LIMITED"
	CodeInsufficientTokens  = "INSUFFICIENT_TOKENS"
	CodeInternal            = "INTERNAL"
	CodeServiceUnavailable  = "SERVICE_UNAVAILABLE"
)

// OK serialises data under {success: true, data: ...} with status 200.
func OK(w http.ResponseWriter, data any) {
	JSON(w, http.StatusOK, Envelope{Success: true, Data: data})
}

// Created is OK with 201.
func Created(w http.ResponseWriter, data any) {
	JSON(w, http.StatusCreated, Envelope{Success: true, Data: data})
}

// Accepted is OK with 202.
func Accepted(w http.ResponseWriter, data any) {
	JSON(w, http.StatusAccepted, Envelope{Success: true, Data: data})
}

// Error serialises an error envelope at the given status.
func Error(w http.ResponseWriter, status int, code, message string) {
	JSON(w, status, Envelope{
		Success: false,
		Error:   &ErrorBody{Code: code, Message: message},
	})
}

// ErrorDetails is Error with structured details (e.g. validation errors).
func ErrorDetails(w http.ResponseWriter, status int, code, message string, details any) {
	JSON(w, status, Envelope{
		Success: false,
		Error:   &ErrorBody{Code: code, Message: message, Details: details},
	})
}

// JSON is the low-level encoder. Sets Content-Type, writes status, then JSON.
func JSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
