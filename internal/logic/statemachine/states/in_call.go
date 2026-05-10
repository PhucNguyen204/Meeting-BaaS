package states

import (
	"context"
	"time"

	"go.uber.org/zap"

	sm "github.com/yourorg/meet-bot-go/internal/logic/statemachine"
	"github.com/yourorg/meet-bot-go/internal/pkg/logger"
)

// InCallState initializes recording resources after the bot has been
// admitted to the meeting.
//
// Port reference: src/state-machine/states/in-call-state.ts.
type InCallState struct {
	Recorder Recorder // injected FFmpeg recorder
}

func (s *InCallState) Name() sm.StateType { return sm.StateInCall }

func (s *InCallState) Execute(ctx context.Context, mc *sm.MeetingContext) (sm.Transition, error) {
	log := logger.FromContext(ctx).With(zap.String("state", "in_call"))
	log.Info("bot is in the meeting, setting up recording")

	// Check for early stop.
	if mc.ShouldStop() {
		log.Info("stop requested before recording, exiting")
		mc.SetError(sm.EndReasonExitBeforeRecord, "Bot was stopped before recording started")
		return sm.Transition{Next: sm.StateCleanup}, nil
	}

	// Start FFmpeg recording.
	if s.Recorder != nil {
		if err := s.Recorder.Start(ctx); err != nil {
			mc.SetError(sm.EndReasonInternal, "failed to start recording: "+err.Error())
			return sm.Transition{Next: sm.StateError}, nil
		}
		log.Info("ffmpeg recording started")
	} else {
		log.Warn("no recorder configured, skipping recording start")
	}

	// Record start time.
	mc.StartTime = time.Now().UnixMilli()
	mc.LastSpeakerTime = time.Now()

	log.Info("transitioning to recording state")
	return sm.Transition{Next: sm.StateRecording}, nil
}
