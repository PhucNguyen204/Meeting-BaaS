// Package statemachine implements the 9-state meeting lifecycle engine.
//
// Port reference: src/state-machine/types.ts + src/state-machine/machine.ts.
package statemachine

import (
	"context"
	"sync"
	"time"

	"github.com/yourorg/meet-bot-go/internal/config"
	"github.com/yourorg/meet-bot-go/internal/logic/browser"
	"github.com/yourorg/meet-bot-go/internal/logic/meeting"
)

// StateType enumerates the state machine's states.
//
// Port reference: src/state-machine/types.ts MeetingStateType.
type StateType string

const (
	StateInitialization StateType = "initialization"
	StateWaitingRoom    StateType = "waitingRoom"
	StateInCall         StateType = "inCall"
	StateRecording      StateType = "recording"
	StatePaused         StateType = "paused"
	StateResuming       StateType = "resuming"
	StateCleanup        StateType = "cleanup"
	StateError          StateType = "error"
	StateTerminated     StateType = "terminated"
)

// EndReason describes why the meeting session ended.
//
// Port reference: src/state-machine/types.ts MeetingEndReason.
type EndReason string

const (
	// Normal end reasons.
	EndReasonBotRemoved           EndReason = "botRemoved"
	EndReasonNoAttendees          EndReason = "noAttendees"
	EndReasonNoSpeaker            EndReason = "noSpeaker"
	EndReasonAllParticipantsLeft  EndReason = "allParticipantsLeft"
	EndReasonRecordingTimeout     EndReason = "recordingTimeout"
	EndReasonApiRequest           EndReason = "apiRequest"
	EndReasonExitBeforeRecord     EndReason = "exitingMeetingBeforeRecord"

	// Error end reasons.
	EndReasonBotRemovedTooEarly   EndReason = "botRemovedTooEarly"
	EndReasonBotNotAccepted       EndReason = "botNotAccepted"
	EndReasonCannotJoinMeeting    EndReason = "cannotJoinMeeting"
	EndReasonTimeoutWaiting       EndReason = "timeoutWaitingToStart"
	EndReasonInvalidMeetingURL    EndReason = "invalidMeetingUrl"
	EndReasonStreamingSetupFailed EndReason = "streamingSetupFailed"
	EndReasonLoginRequired        EndReason = "loginRequired"
	EndReasonInternal             EndReason = "internalError"
)

// NormalEndReasons is the set of reasons considered a successful recording.
//
// Port reference: src/state-machine/constants.ts NORMAL_END_REASONS.
var NormalEndReasons = map[EndReason]bool{
	EndReasonBotRemoved:          true,
	EndReasonNoAttendees:         true,
	EndReasonNoSpeaker:           true,
	EndReasonAllParticipantsLeft: true,
	EndReasonRecordingTimeout:    true,
	EndReasonApiRequest:          true,
}

// ErrorMessage returns a human-readable description for a given end reason.
//
// Port reference: src/state-machine/types.ts getErrorMessageFromCode().
func (r EndReason) ErrorMessage() string {
	switch r {
	case EndReasonBotRemoved:
		return "Bot was removed from the meeting."
	case EndReasonNoAttendees:
		return "No attendees joined the meeting."
	case EndReasonNoSpeaker:
		return "No speakers detected during recording."
	case EndReasonAllParticipantsLeft:
		return "All participants left the meeting."
	case EndReasonRecordingTimeout:
		return "Recording timeout reached."
	case EndReasonApiRequest:
		return "Recording stopped via API request."
	case EndReasonExitBeforeRecord:
		return "Bot exited before recording started."
	case EndReasonBotRemovedTooEarly:
		return "Bot was removed too early; the video is too short."
	case EndReasonBotNotAccepted:
		return "Bot was not accepted into the meeting."
	case EndReasonCannotJoinMeeting:
		return "Cannot join meeting - meeting is not reachable."
	case EndReasonTimeoutWaiting:
		return "Timeout waiting to start recording."
	case EndReasonInvalidMeetingURL:
		return "Invalid meeting URL provided."
	case EndReasonStreamingSetupFailed:
		return "Failed to set up streaming audio."
	case EndReasonLoginRequired:
		return "Login required to access the meeting."
	case EndReasonInternal:
		return "Internal error occurred during recording."
	default:
		return "An error occurred during recording."
	}
}

// IsNormal reports whether this end reason represents a successful session.
func (r EndReason) IsNormal() bool {
	return NormalEndReasons[r]
}

// MeetingContext holds the mutable runtime state of a meeting session.
//
// This replaces the TS singleton pattern (GLOBAL + MeetingContext interface).
// Fields are guarded by mu for concurrent access from the state machine loop,
// the HTTP server goroutine, and the Redis subscriber goroutine.
//
// Port reference: src/state-machine/types.ts MeetingContext +
//
//	src/singleton.ts GLOBAL.
type MeetingContext struct {
	mu sync.RWMutex

	// Config is the immutable session configuration.
	Config *config.BotConfig

	// Provider is the meeting platform implementation.
	Provider meeting.Provider

	// Browser holds the playwright driver managing the chromium instance.
	BrowserDriver browser.Driver

	// Page is the active meeting page (set by Initialization state).
	Page browser.Page

	// --- Timing ---
	StartTime             int64     // UNIX ms when recording actually started
	LastSpeakerTime       time.Time // last time any speaker was detected
	NoSpeakerDetectedTime time.Time // when the "no speaker" timer started

	// --- Participant tracking ---
	AttendeesCount   int
	FirstUserJoined  bool

	// --- Recording state (pause/resume) ---
	IsPaused           bool
	PauseStartTime     time.Time
	TotalPauseDuration time.Duration

	// --- End state ---
	EndReason    EndReason
	ErrorMessage string
	HasError     bool
}

// SetEndReason atomically sets the end reason. Thread-safe.
//
// Port reference: src/singleton.ts GLOBAL.setEndReason().
func (mc *MeetingContext) SetEndReason(reason EndReason) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.EndReason = reason
}

// GetEndReason atomically reads the end reason.
func (mc *MeetingContext) GetEndReason() EndReason {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return mc.EndReason
}

// SetError atomically sets the error state.
//
// Port reference: src/singleton.ts GLOBAL.setError().
func (mc *MeetingContext) SetError(reason EndReason, msg string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.EndReason = reason
	mc.ErrorMessage = msg
	mc.HasError = true
}

// GetError atomically reads the error state.
func (mc *MeetingContext) GetError() (bool, EndReason, string) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return mc.HasError, mc.EndReason, mc.ErrorMessage
}

// ShouldStop reports whether a stop request has been issued. Called by
// recording state's polling loop.
func (mc *MeetingContext) ShouldStop() bool {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return mc.EndReason != ""
}

// State is the interface every state implementation must satisfy.
//
// Port reference: src/state-machine/states/base-state.ts BaseState.
type State interface {
	// Name returns the state's type identifier.
	Name() StateType

	// Execute runs the state's logic. Returns the next state to transition to.
	// The MeetingContext is mutated in place (mutex-guarded fields).
	Execute(ctx context.Context, mc *MeetingContext) (Transition, error)
}

// Transition carries the result of a state execution.
//
// Port reference: src/state-machine/types.ts StateTransition.
type Transition struct {
	Next StateType
}
