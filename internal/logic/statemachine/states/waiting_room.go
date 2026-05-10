package states

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/yourorg/meet-bot-go/internal/logic/meeting"
	sm "github.com/yourorg/meet-bot-go/internal/logic/statemachine"
	"github.com/yourorg/meet-bot-go/internal/pkg/logger"
	"github.com/yourorg/meet-bot-go/internal/pkg/sleep"
)

// WaitingRoomState handles the join flow: typing bot name, clicking
// "Ask to join", waiting for host acceptance, timeout handling.
//
// Port reference: src/state-machine/states/waiting-room-state.ts.
type WaitingRoomState struct{}

func (s *WaitingRoomState) Name() sm.StateType { return sm.StateWaitingRoom }

func (s *WaitingRoomState) Execute(ctx context.Context, mc *sm.MeetingContext) (sm.Transition, error) {
	log := logger.FromContext(ctx).With(zap.String("state", "waiting_room"))
	log.Info("starting join flow")

	if mc.Page == nil || mc.Provider == nil {
		mc.SetError(sm.EndReasonInternal, "waiting_room: nil page or provider")
		return sm.Transition{Next: sm.StateError}, nil
	}

	// Build join options from config.
	timeout := time.Duration(mc.Config.AutomaticLeave.WaitingRoomTimeout) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Minute // default
	}

	joinCtx, joinCancel := context.WithTimeout(ctx, timeout)
	defer joinCancel()

	joinOpts := meeting.JoinOptions{
		BotName:       mc.Config.BotName,
		EnterMessage:  mc.Config.EnterMessage,
		StartTimeUnix: mc.Config.StartTime,
		CancelCheck: func() bool {
			return mc.ShouldStop()
		},
		OnJoinSuccess: func() {
			log.Info("join success callback fired")
			mc.FirstUserJoined = true
		},
	}

	err := mc.Provider.JoinMeeting(joinCtx, mc.Page, joinOpts)
	if err != nil {
		// Check if it was a stop request vs a real error.
		if mc.ShouldStop() {
			log.Info("join aborted by stop request")
			return sm.Transition{Next: sm.StateCleanup}, nil
		}

		// Classify the error.
		if joinCtx.Err() != nil {
			mc.SetError(sm.EndReasonTimeoutWaiting, "waiting room timeout exceeded")
		} else {
			mc.SetError(sm.EndReasonBotNotAccepted, err.Error())
		}
		return sm.Transition{Next: sm.StateError}, nil
	}

	// Brief stabilization period after join.
	_ = sleep.For(ctx, 500*time.Millisecond)

	log.Info("joined meeting, transitioning to in_call")
	return sm.Transition{Next: sm.StateInCall}, nil
}
