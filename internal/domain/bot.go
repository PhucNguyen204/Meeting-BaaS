package domain

import (
	"context"
	"go.uber.org/zap"
	"time"
)

// BotState represents the current state of the bot in the meeting lifecycle.
type BotState string

const (
	StateInitialization BotState = "Initialization"
	StateWaitingRoom    BotState = "WaitingRoom"
	StateInCall         BotState = "InCall"
	StateRecording      BotState = "Recording"
	StatePaused         BotState = "Paused"
	StateCleanup        BotState = "Cleanup"
	StateTerminated     BotState = "Terminated"
	StateError          BotState = "Error"
)

// BotContext holds all the dependencies and state needed by the state machine
// and the individual states. It acts as the central registry.
type BotContext interface {
	context.Context

	Log() *zap.Logger
	Config() interface{} // Usually internal/config.BotConfig

	Browser() BrowserDriver
	Page() Page
	SetPage(page Page)

	Recorder() Recorder
	Webhook() WebhookSender
	Provider() MeetingProvider

	MeetingInfo() MeetingInfo
	SetMeetingInfo(info MeetingInfo)

	// State transitions
	TransitionTo(newState BotState)
	CurrentState() BotState
	RecordError(err error)
	LastError() error

	// Lifecycle
	StartedAt() time.Time
}
