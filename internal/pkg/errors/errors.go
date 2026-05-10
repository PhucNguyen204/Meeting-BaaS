// Package errors centralises sentinel errors used across the bot pipeline.
//
// Sentinels here roughly mirror the [MeetingEndReason] enum from
// [src/state-machine/types.ts:20-41]. The state machine package will define
// its own typed enum (Phase 2); these sentinels exist for adapter packages
// (browser, meeting/meet, dialog, ...) so they can return semantic errors
// without depending on the state machine package and creating an import cycle.
//
// Use errors.Is to check:
//
//	if errors.Is(err, perrors.ErrBotNotAccepted) { ... }
package errors

import "errors"

// Re-export stdlib helpers so callers only need this package.
var (
	Is     = errors.Is
	As     = errors.As
	New    = errors.New
	Unwrap = errors.Unwrap
)

// Wrap is a thin alias for fmt.Errorf("%s: %w", msg, err) without requiring
// the fmt import at call sites. Returns nil if err is nil.
//
// Prefer fmt.Errorf at call sites where extra interpolation is needed; this
// helper exists for the simple "tag with context, preserve cause" case.
func Wrap(msg string, err error) error {
	if err == nil {
		return nil
	}
	return errWrap{msg: msg, err: err}
}

type errWrap struct {
	msg string
	err error
}

func (e errWrap) Error() string { return e.msg + ": " + e.err.Error() }
func (e errWrap) Unwrap() error { return e.err }

// Sentinel errors. These mirror MeetingEndReason values.
//
// Port reference: src/state-machine/types.ts MeetingEndReason.
var (
	// ErrInvalidMeetingURL is returned when the URL is missing/malformed
	// or points at an unsupported provider.
	ErrInvalidMeetingURL = errors.New("invalid meeting url")

	// ErrCannotJoinMeeting indicates the meeting page returned a non-2xx
	// or otherwise refused to load (Meet 5xx, network errors).
	ErrCannotJoinMeeting = errors.New("cannot join meeting")

	// ErrBotNotAccepted indicates the host denied the bot or Google Meet
	// auto-redirected away from meet.google.com.
	ErrBotNotAccepted = errors.New("bot not accepted")

	// ErrBotRemoved indicates the bot was removed from a call it had joined.
	ErrBotRemoved = errors.New("bot removed")

	// ErrTimeoutWaitingToStart fires when waiting_room_timeout elapses.
	ErrTimeoutWaitingToStart = errors.New("timeout waiting to start")

	// ErrLoginRequired indicates the meeting requires Google sign-in,
	// which serverless bots cannot satisfy.
	ErrLoginRequired = errors.New("login required")

	// ErrApiRequest indicates the operator stopped the bot via /stop_record.
	ErrApiRequest = errors.New("stopped via api request")

	// ErrExitingBeforeRecord indicates a stop request arrived before the
	// recorder started — used to suppress webhook noise.
	ErrExitingBeforeRecord = errors.New("exiting before record")

	// ErrStreamingSetupFailed wraps audio/video pipeline setup errors.
	ErrStreamingSetupFailed = errors.New("streaming setup failed")

	// ErrInternal is the catch-all for unexpected failures.
	ErrInternal = errors.New("internal error")
)
