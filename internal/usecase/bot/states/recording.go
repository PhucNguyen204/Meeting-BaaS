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

	// firstUserGracePeriod is how long we wait at the very start of recording
	// before considering "alone in meeting" — the bot is the only attendee
	// for a brief window during the join. Mirrors recording-state.ts.
	firstUserGracePeriod = 30 * time.Second
)

// RecordingState is the main active-recording loop. Polls for end-meeting
// signals, silence timeouts, and stop requests.
//
// Port reference: src/state-machine/states/recording-state.ts.
type RecordingState struct {
	// Speakers, when non-nil, is queried each tick to update
	// MeetingContext.AttendeesCount and MeetingContext.LastSpeakerTime.
	Speakers SpeakerSnapshot
}

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
		if mc.GetPaused() {
			log.Info("pause requested, transitioning to paused")
			return sm.Transition{Next: sm.StatePaused}, nil
		}

		// 4. Maximum recording duration guard.
		if time.Since(recordStart) >= maxRecordingDuration {
			log.Warn("maximum recording duration exceeded")
			mc.SetEndReason(sm.EndReasonRecordingTimeout)
			return sm.Transition{Next: sm.StateCleanup}, nil
		}

		// 5. Update speaker / attendee tracking from observer.
		s.updateAttendees(mc)

		// 6. Check if removed from meeting.
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

		// 7. Silence timeout check.
		if reason, ok := s.checkNoSpeaker(mc, recordStart); ok {
			log.Warn("no-speaker timeout", zap.String("reason", string(reason)))
			mc.SetEndReason(reason)
			return sm.Transition{Next: sm.StateCleanup}, nil
		}

		// 8. No-one-joined timeout (before any attendee ever joined).
		if reason, ok := s.checkNoOneJoined(mc, recordStart); ok {
			log.Warn("no-one-joined timeout", zap.String("reason", string(reason)))
			mc.SetEndReason(reason)
			return sm.Transition{Next: sm.StateCleanup}, nil
		}

		// 9. Alone-in-meeting check (after others have joined and left).
		if reason, ok := s.checkAloneInMeeting(mc, recordStart); ok {
			log.Warn("alone-in-meeting", zap.String("reason", string(reason)))
			mc.SetEndReason(reason)
			return sm.Transition{Next: sm.StateCleanup}, nil
		}

		_ = sleep.For(ctx, recordingPollInterval)
	}
}

// updateAttendees queries the speaker manager and updates mc fields:
//   - AttendeesCount = len(allParticipants) - 1 (subtract bot itself if present)
//   - FirstUserJoined = true once attendees ≥ 1 and a non-bot name appears
//   - LastSpeakerTime = now if any active speaker exists
//   - NoSpeakerDetectedTime: starts when active goes empty, resets when active
//
// Port reference: recording-state.ts updateAttendeesAndSpeakers().
func (s *RecordingState) updateAttendees(mc *sm.MeetingContext) {
	if s == nil || s.Speakers == nil {
		return
	}
	active, all := s.Speakers.SnapshotNames()

	botName := mc.Config.BotName
	otherCount := 0
	for _, n := range all {
		if n != "" && n != botName {
			otherCount++
		}
	}
	mc.AttendeesCount = otherCount
	if otherCount > 0 {
		mc.FirstUserJoined = true
	}

	now := time.Now()
	hasActive := false
	for _, n := range active {
		if n != "" && n != botName {
			hasActive = true
			break
		}
	}
	if hasActive {
		mc.LastSpeakerTime = now
		mc.NoSpeakerDetectedTime = time.Time{}
	} else if mc.NoSpeakerDetectedTime.IsZero() && mc.FirstUserJoined {
		// First time we observe silence after someone has joined → start the timer.
		mc.NoSpeakerDetectedTime = now
	}
}

// checkNoSpeaker fires when no participant has been an active speaker for
// SilenceTimeout seconds since either recording start (if no one ever spoke)
// or the last speaker event.
//
// Port reference: recording-state.ts checkNoSpeaker().
func (s *RecordingState) checkNoSpeaker(mc *sm.MeetingContext, recordStart time.Time) (sm.EndReason, bool) {
	timeout := time.Duration(mc.Config.AutomaticLeave.SilenceTimeout) * time.Second
	if timeout <= 0 {
		return "", false
	}
	// Never trigger before someone joined; that case is handled by checkNoOneJoined.
	if !mc.FirstUserJoined {
		return "", false
	}
	if mc.NoSpeakerDetectedTime.IsZero() {
		return "", false
	}
	if time.Since(mc.NoSpeakerDetectedTime) >= timeout {
		// Skip the very first grace period after recordStart.
		if time.Since(recordStart) < firstUserGracePeriod {
			return "", false
		}
		return sm.EndReasonNoSpeaker, true
	}
	return "", false
}

// checkNoOneJoined fires when nobody else ever joined for NooneJoinedTimeout
// seconds since recording start.
//
// Port reference: recording-state.ts checkNoOneJoined().
func (s *RecordingState) checkNoOneJoined(mc *sm.MeetingContext, recordStart time.Time) (sm.EndReason, bool) {
	timeout := time.Duration(mc.Config.AutomaticLeave.NooneJoinedTimeout) * time.Second
	if timeout <= 0 {
		return "", false
	}
	if mc.FirstUserJoined {
		return "", false
	}
	if time.Since(recordStart) >= timeout {
		return sm.EndReasonNoAttendees, true
	}
	return "", false
}

// checkAloneInMeeting fires when participants joined at some point but have
// all left, leaving the bot alone, for at least the silence-timeout window.
//
// Port reference: recording-state.ts checkAloneInMeeting().
func (s *RecordingState) checkAloneInMeeting(mc *sm.MeetingContext, recordStart time.Time) (sm.EndReason, bool) {
	if !mc.FirstUserJoined {
		return "", false
	}
	if mc.AttendeesCount > 0 {
		return "", false
	}
	if time.Since(recordStart) < firstUserGracePeriod {
		return "", false
	}
	timeout := time.Duration(mc.Config.AutomaticLeave.NooneJoinedTimeout) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	if mc.NoSpeakerDetectedTime.IsZero() {
		return "", false
	}
	if time.Since(mc.NoSpeakerDetectedTime) >= timeout {
		return sm.EndReasonAllParticipantsLeft, true
	}
	return "", false
}
