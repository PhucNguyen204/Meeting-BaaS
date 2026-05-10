package states

import (
	"github.com/PhucNguyen204/Meeting-BaaS/internal/domain"
	sm "github.com/PhucNguyen204/Meeting-BaaS/internal/usecase/bot"

	"github.com/PhucNguyen204/Meeting-BaaS/internal/infra/meeting"
)

// Dependencies holds all injected dependencies needed to build the
// complete state map. Follows the Composition Root pattern.
type Dependencies struct {
	Driver      domain.BrowserDriver
	Provider    meeting.Provider
	BrowserOpts domain.BrowserLaunchOptions
	Recorder    Recorder  // nil = no recording
	Uploader    Uploader  // nil = no upload
	Webhooker   Webhooker // nil = no webhooks
}

// BuildStateMap constructs the full state machine state map from deps.
//
// This is the single place where all states are wired together. The
// composition root (internal/app) calls this once during setup.
func BuildStateMap(deps Dependencies) map[sm.StateType]sm.State {
	return map[sm.StateType]sm.State{
		sm.StateInitialization: &InitializationState{
			Driver:   deps.Driver,
			Provider: deps.Provider,
			Opts:     deps.BrowserOpts,
		},
		sm.StateWaitingRoom: &WaitingRoomState{},
		sm.StateInCall: &InCallState{
			Recorder: deps.Recorder,
		},
		sm.StateRecording: &RecordingState{},
		sm.StatePaused: &PausedState{
			Recorder: deps.Recorder,
		},
		sm.StateResuming: &ResumingState{
			Recorder: deps.Recorder,
		},
		sm.StateCleanup: &CleanupState{
			Recorder:  deps.Recorder,
			Uploader:  deps.Uploader,
			Webhooker: deps.Webhooker,
		},
		sm.StateError: &ErrorState{
			Webhooker: deps.Webhooker,
		},
		sm.StateTerminated: &TerminatedState{},
	}
}
