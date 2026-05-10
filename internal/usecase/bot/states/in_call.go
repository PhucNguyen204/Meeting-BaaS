package states

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/PhucNguyen204/Meeting-BaaS/internal/pkg/logger"
	sm "github.com/PhucNguyen204/Meeting-BaaS/internal/usecase/bot"
)

// InCallState initializes recording resources after the bot has been
// admitted to the meeting.
//
// Port reference: src/state-machine/states/in-call-state.ts.
type InCallState struct {
	Recorder  Recorder   // injected FFmpeg recorder
	PageHooks []PageHook // audio capture, speakers observer, dialog observer
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

	// Attach page-bound observers (audio capture, speakers, dialog). Failures
	// are logged but don't abort the session — recording is still useful even
	// without the speaker timeline.
	for _, hook := range s.PageHooks {
		if hook == nil || mc.Page == nil {
			continue
		}
		if err := hook.Attach(ctx, mc.Page); err != nil {
			log.Warn("page hook attach failed", zap.Error(err))
		}
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

	// Record start time (thread-safe; HTTP /status reads via GetStartTime).
	mc.SetStartTime(time.Now().UnixMilli())
	mc.LastSpeakerTime = time.Now()

	log.Info("transitioning to recording state")
	return sm.Transition{Next: sm.StateRecording}, nil
}
