// Package statemachine — machine.go implements the state machine engine.
//
// Port reference: src/state-machine/machine.ts MeetingStateMachine.
package statemachine

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/yourorg/meet-bot-go/internal/pkg/logger"
)

// Machine drives the meeting lifecycle through its 9 states.
//
// Port reference: src/state-machine/machine.ts MeetingStateMachine.
type Machine struct {
	current StateType
	mc      *MeetingContext
	states  map[StateType]State
	log     *zap.Logger
}

// NewMachine constructs a Machine with the given meeting context and states.
//
// stateMap must contain at least StateInitialization and StateTerminated.
func NewMachine(mc *MeetingContext, stateMap map[StateType]State, log *zap.Logger) *Machine {
	return &Machine{
		current: StateInitialization,
		mc:      mc,
		states:  stateMap,
		log:     log,
	}
}

// Run executes the state machine loop until StateTerminated is reached or
// the context is cancelled.
//
// Port reference: src/state-machine/machine.ts MeetingStateMachine.start().
func (m *Machine) Run(ctx context.Context) error {
	ctx = logger.IntoContext(ctx, m.log)
	log := logger.FromContext(ctx)

	for m.current != StateTerminated {
		if ctx.Err() != nil {
			log.Warn("context cancelled, forcing cleanup",
				zap.String("state", string(m.current)),
				zap.Error(ctx.Err()),
			)
			m.current = StateTerminated
			break
		}

		log.Info("state machine transition",
			zap.String("state", string(m.current)),
		)

		state, ok := m.states[m.current]
		if !ok {
			return fmt.Errorf("statemachine: no handler registered for state %q", m.current)
		}

		transition, err := state.Execute(ctx, m.mc)
		if err != nil {
			log.Error("state execution error",
				zap.String("state", string(m.current)),
				zap.Error(err),
			)
			// Set error on meeting context and transition to Error state,
			// unless we're already in Error or Cleanup (avoid infinite loops).
			if m.current != StateError && m.current != StateCleanup {
				m.mc.SetError(EndReasonInternal, err.Error())
				m.current = StateError
				continue
			}
			// If Error or Cleanup itself fails, force terminated.
			m.current = StateTerminated
			continue
		}

		m.current = transition.Next
	}

	log.Info("state machine terminated",
		zap.String("end_reason", string(m.mc.GetEndReason())),
	)
	return nil
}

// CurrentState returns the current state type. Thread-safe for reads from
// the HTTP handler goroutine.
func (m *Machine) CurrentState() StateType {
	return m.current
}

// RequestStop sets the end reason on the meeting context, which the
// recording state's polling loop will detect.
//
// Port reference: src/state-machine/machine.ts requestStop().
func (m *Machine) RequestStop(reason EndReason) {
	m.log.Info("stop requested", zap.String("reason", string(reason)))

	// If the bot hasn't started recording yet, use ExitBeforeRecord.
	preRecording := m.current == StateInitialization ||
		m.current == StateWaitingRoom ||
		m.current == StateInCall
	if reason == EndReasonApiRequest && preRecording {
		m.log.Info("bot is in pre-recording state, using ExitBeforeRecord",
			zap.String("current_state", string(m.current)),
		)
		m.mc.SetError(EndReasonExitBeforeRecord, "Bot was stopped before recording started")
		return
	}

	m.mc.SetEndReason(reason)
}

// WasRecordingSuccessful reports whether the session ended with a normal
// end reason (i.e., a recording was produced).
//
// Port reference: src/state-machine/machine.ts wasRecordingSuccessful().
func (m *Machine) WasRecordingSuccessful() bool {
	hasErr, _, _ := m.mc.GetError()
	if hasErr {
		return false
	}
	reason := m.mc.GetEndReason()
	return reason != "" && reason.IsNormal()
}
