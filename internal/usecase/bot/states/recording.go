package states

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/PhucNguyen204/Meeting-BaaS/internal/pkg/logger"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/pkg/sleep"
	sm "github.com/PhucNguyen204/Meeting-BaaS/internal/usecase/bot"
)

const (
	recordingPollInterval = 2 * time.Second
	maxRecordingDuration  = 4 * time.Hour
)

// RecordingState is the main active-recording loop. Polls for end-meeting
// signals, silence timeouts, and stop requests.
//
// Port reference: src/state-machine/states/recording-state.ts.
type RecordingState struct{}

func (s *RecordingState) Name() sm.StateType { return sm.StateRecording }

func (s *RecordingState) Execute(ctx context.Context, mc *sm.MeetingContext) (sm.Transition, error) {
	log := logger.FromContext(ctx).With(zap.String("state", "recording"))
	log.Info("recording active, polling for end conditions")

	recordStart := time.Now()

	for {
		// 1. Context cancelled (SIGTERM, parent shutdown).
		if ctx.Err() != nil {
			mc.SetEndReason(sm.EndReasonApiRequest)
			return sm.Transition{Next: sm.StateCleanup}, nil
		}

		// 2. Stop requested via HTTP / Redis.
		if mc.ShouldStop() {
			log.Info("stop request detected", zap.String("reason", string(mc.GetEndReason())))
			return sm.Transition{Next: sm.StateCleanup}, nil
		}

		// 3. Pause requested.
		if mc.IsPaused {
			log.Info("pause requested, transitioning to paused")
			return sm.Transition{Next: sm.StatePaused}, nil
		}

		// 4. Maximum recording duration guard.
		if time.Since(recordStart) >= maxRecordingDuration {
			log.Warn("maximum recording duration exceeded")
			mc.SetEndReason(sm.EndReasonRecordingTimeout)
			return sm.Transition{Next: sm.StateCleanup}, nil
		}

		// 5. Check if removed from meeting.
		if mc.Provider != nil && mc.Page != nil {
			removed, err := mc.Provider.FindEndMeeting(ctx, mc.Page)
			if err != nil {
				log.Warn("findEndMeeting error", zap.Error(err))
			} else if removed {
				log.Info("bot removed from meeting")
				mc.SetEndReason(sm.EndReasonBotRemoved)
				return sm.Transition{Next: sm.StateCleanup}, nil
			}
		}

		// 6. Silence timeout check.
		silenceTimeout := time.Duration(mc.Config.AutomaticLeave.SilenceTimeout) * time.Second
		if silenceTimeout > 0 && !mc.NoSpeakerDetectedTime.IsZero() {
			if time.Since(mc.NoSpeakerDetectedTime) >= silenceTimeout {
				log.Warn("silence timeout reached",
					zap.Duration("silence", time.Since(mc.NoSpeakerDetectedTime)),
				)
				mc.SetEndReason(sm.EndReasonNoSpeaker)
				return sm.Transition{Next: sm.StateCleanup}, nil
			}
		}

		// 7. No attendees timeout.
		nooneTimeout := time.Duration(mc.Config.AutomaticLeave.NooneJoinedTimeout) * time.Second
		if nooneTimeout > 0 && mc.AttendeesCount == 0 && mc.FirstUserJoined {
			log.Warn("all participants left")
			mc.SetEndReason(sm.EndReasonAllParticipantsLeft)
			return sm.Transition{Next: sm.StateCleanup}, nil
		}

		_ = sleep.For(ctx, recordingPollInterval)
	}
}
