package states

import (
	"context"

	sm "github.com/yourorg/meet-bot-go/internal/logic/statemachine"
)

// TerminatedState is the terminal no-op state. The state machine loop
// exits when it reaches this state.
//
// Port reference: implicit in src/state-machine/machine.ts start() loop.
type TerminatedState struct{}

func (s *TerminatedState) Name() sm.StateType { return sm.StateTerminated }

func (s *TerminatedState) Execute(_ context.Context, _ *sm.MeetingContext) (sm.Transition, error) {
	// This should never be called — the machine loop exits before executing
	// the terminated state. Included for completeness so the registry is
	// always complete.
	return sm.Transition{Next: sm.StateTerminated}, nil
}
