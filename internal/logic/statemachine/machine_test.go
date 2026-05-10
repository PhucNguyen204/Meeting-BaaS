package statemachine_test

import (
	"context"
	"testing"

	"go.uber.org/zap"

	sm "github.com/yourorg/meet-bot-go/internal/logic/statemachine"
)

// mockState is a test helper that transitions to a fixed next state.
type mockState struct {
	name    sm.StateType
	next    sm.StateType
	execFn  func(ctx context.Context, mc *sm.MeetingContext) (sm.Transition, error)
}

func (s *mockState) Name() sm.StateType { return s.name }
func (s *mockState) Execute(ctx context.Context, mc *sm.MeetingContext) (sm.Transition, error) {
	if s.execFn != nil {
		return s.execFn(ctx, mc)
	}
	return sm.Transition{Next: s.next}, nil
}

// TestMachine_HappyPath verifies the full state lifecycle:
// Initialization -> WaitingRoom -> InCall -> Recording -> Cleanup -> Terminated
func TestMachine_HappyPath(t *testing.T) {
	var visitOrder []sm.StateType

	makeState := func(name, next sm.StateType) sm.State {
		return &mockState{
			name: name,
			next: next,
			execFn: func(_ context.Context, _ *sm.MeetingContext) (sm.Transition, error) {
				visitOrder = append(visitOrder, name)
				return sm.Transition{Next: next}, nil
			},
		}
	}

	states := map[sm.StateType]sm.State{
		sm.StateInitialization: makeState(sm.StateInitialization, sm.StateWaitingRoom),
		sm.StateWaitingRoom:    makeState(sm.StateWaitingRoom, sm.StateInCall),
		sm.StateInCall:         makeState(sm.StateInCall, sm.StateRecording),
		sm.StateRecording:      makeState(sm.StateRecording, sm.StateCleanup),
		sm.StateCleanup:        makeState(sm.StateCleanup, sm.StateTerminated),
	}

	mc := &sm.MeetingContext{}
	log := zap.NewNop()
	machine := sm.NewMachine(mc, states, log)

	err := machine.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []sm.StateType{
		sm.StateInitialization,
		sm.StateWaitingRoom,
		sm.StateInCall,
		sm.StateRecording,
		sm.StateCleanup,
	}

	if len(visitOrder) != len(expected) {
		t.Fatalf("expected %d states visited, got %d: %v", len(expected), len(visitOrder), visitOrder)
	}
	for i, s := range expected {
		if visitOrder[i] != s {
			t.Errorf("state %d: expected %s, got %s", i, s, visitOrder[i])
		}
	}
}

// TestMachine_ErrorRecovery verifies that a state error transitions to Error
// then to Cleanup then to Terminated.
func TestMachine_ErrorRecovery(t *testing.T) {
	var visitOrder []sm.StateType

	states := map[sm.StateType]sm.State{
		sm.StateInitialization: &mockState{
			name: sm.StateInitialization,
			execFn: func(_ context.Context, _ *sm.MeetingContext) (sm.Transition, error) {
				visitOrder = append(visitOrder, sm.StateInitialization)
				return sm.Transition{}, context.DeadlineExceeded // simulate error
			},
		},
		sm.StateError: &mockState{
			name: sm.StateError,
			execFn: func(_ context.Context, _ *sm.MeetingContext) (sm.Transition, error) {
				visitOrder = append(visitOrder, sm.StateError)
				return sm.Transition{Next: sm.StateCleanup}, nil
			},
		},
		sm.StateCleanup: &mockState{
			name: sm.StateCleanup,
			execFn: func(_ context.Context, _ *sm.MeetingContext) (sm.Transition, error) {
				visitOrder = append(visitOrder, sm.StateCleanup)
				return sm.Transition{Next: sm.StateTerminated}, nil
			},
		},
	}

	mc := &sm.MeetingContext{}
	log := zap.NewNop()
	machine := sm.NewMachine(mc, states, log)

	err := machine.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []sm.StateType{
		sm.StateInitialization,
		sm.StateError,
		sm.StateCleanup,
	}
	if len(visitOrder) != len(expected) {
		t.Fatalf("expected %d states visited, got %d: %v", len(expected), len(visitOrder), visitOrder)
	}
	for i, s := range expected {
		if visitOrder[i] != s {
			t.Errorf("state %d: expected %s, got %s", i, s, visitOrder[i])
		}
	}

	// Verify error was recorded
	hasErr, reason, _ := mc.GetError()
	if !hasErr {
		t.Error("expected error to be set on context")
	}
	if reason != sm.EndReasonInternal {
		t.Errorf("expected end reason %s, got %s", sm.EndReasonInternal, reason)
	}
}

// TestMachine_RequestStop verifies that RequestStop correctly sets end reason.
func TestMachine_RequestStop(t *testing.T) {
	mc := &sm.MeetingContext{}
	log := zap.NewNop()

	states := map[sm.StateType]sm.State{
		sm.StateInitialization: &mockState{name: sm.StateInitialization, next: sm.StateTerminated},
	}
	machine := sm.NewMachine(mc, states, log)

	// Test pre-recording stop converts to ExitBeforeRecord
	machine.RequestStop(sm.EndReasonApiRequest)
	hasErr, reason, _ := mc.GetError()
	if !hasErr {
		t.Error("expected error to be set for pre-recording stop")
	}
	if reason != sm.EndReasonExitBeforeRecord {
		t.Errorf("expected %s, got %s", sm.EndReasonExitBeforeRecord, reason)
	}
}

// TestEndReason_IsNormal verifies the normal end reason classification.
func TestEndReason_IsNormal(t *testing.T) {
	normalReasons := []sm.EndReason{
		sm.EndReasonBotRemoved,
		sm.EndReasonNoAttendees,
		sm.EndReasonNoSpeaker,
		sm.EndReasonAllParticipantsLeft,
		sm.EndReasonRecordingTimeout,
		sm.EndReasonApiRequest,
	}
	for _, r := range normalReasons {
		if !r.IsNormal() {
			t.Errorf("expected %s to be normal", r)
		}
	}

	errorReasons := []sm.EndReason{
		sm.EndReasonBotNotAccepted,
		sm.EndReasonCannotJoinMeeting,
		sm.EndReasonInvalidMeetingURL,
		sm.EndReasonInternal,
	}
	for _, r := range errorReasons {
		if r.IsNormal() {
			t.Errorf("expected %s to NOT be normal", r)
		}
	}
}
