package states

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/PhucNguyen204/Meeting-BaaS/internal/pkg/logger"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/pkg/sleep"
	sm "github.com/PhucNguyen204/Meeting-BaaS/internal/usecase/bot"
)

// PausedState suspends recording and waits for a resume signal.
//
// Port reference: src/state-machine/states/paused-state.ts.
type PausedState struct {
	Recorder Recorder
}

func (s *PausedState) Name() sm.StateType { return sm.StatePaused }

func (s *PausedState) Execute(ctx context.Context, mc *sm.MeetingContext) (sm.Transition, error) {
	log := logger.FromContext(ctx).With(zap.String("state", "paused"))
	log.Info("recording paused")

	mc.PauseStartTime = time.Now()

	if s.Recorder != nil {
		if err := s.Recorder.Pause(ctx); err != nil {
			log.Warn("recorder pause failed", zap.Error(err))
		}
	}

	// Poll for resume or stop.
	for {
		if ctx.Err() != nil {
			mc.SetEndReason(sm.EndReasonApiRequest)
			return sm.Transition{Next: sm.StateCleanup}, nil
		}
		if mc.ShouldStop() {
			return sm.Transition{Next: sm.StateCleanup}, nil
		}
		if !mc.IsPaused {
			log.Info("resume signal received")
			return sm.Transition{Next: sm.StateResuming}, nil
		}
		_ = sleep.For(ctx, 500*time.Millisecond)
	}
}

// ResumingState resumes recording from a paused state.
//
// Port reference: src/state-machine/states/resuming-state.ts.
type ResumingState struct {
	Recorder Recorder
}

func (s *ResumingState) Name() sm.StateType { return sm.StateResuming }

func (s *ResumingState) Execute(ctx context.Context, mc *sm.MeetingContext) (sm.Transition, error) {
	log := logger.FromContext(ctx).With(zap.String("state", "resuming"))
	log.Info("resuming recording")

	// Calculate total pause duration.
	if !mc.PauseStartTime.IsZero() {
		mc.TotalPauseDuration += time.Since(mc.PauseStartTime)
		mc.PauseStartTime = time.Time{}
	}

	if s.Recorder != nil {
		if err := s.Recorder.Resume(ctx); err != nil {
			mc.SetError(sm.EndReasonInternal, "resume recording failed: "+err.Error())
			return sm.Transition{Next: sm.StateError}, nil
		}
	}

	log.Info("recording resumed", zap.Duration("total_pause", mc.TotalPauseDuration))
	return sm.Transition{Next: sm.StateRecording}, nil
}
