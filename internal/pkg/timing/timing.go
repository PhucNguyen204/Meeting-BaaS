// Package timing implements the start_time gate from
// [src/utils/timing-control.ts].
//
// The bot may be scheduled to join a meeting at a specific UNIX timestamp
// (BotConfig.StartTime). HandleTimingControl blocks until that moment,
// while polling an optional abort callback so we can bail out if the
// meeting is cancelled / the page is redirected away during the wait.
package timing

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/PhucNguyen204/Meeting-BaaS/internal/pkg/logger"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/pkg/sleep"
)

// AbortCheck returns true to short-circuit the wait. Callers typically
// poll the meeting page URL to detect Google's "you can't join" auto-redirect.
//
// Port reference: src/utils/timing-control.ts and the Meet abort callback
// in [src/state-machine/states/waiting-room-state.ts:158-171].
type AbortCheck func(ctx context.Context) (bool, error)

// HandleTimingControl returns the actual join timestamp (in milliseconds
// since epoch).
//
//   - If startTimeUnix is 0 or in the past, returns now() immediately.
//   - Otherwise, sleeps until startTimeUnix while invoking abort every pollEvery.
//
// The returned timestamp is used by the recorder to crop the video / audio
// to the scheduled join time so accidental early starts don't pollute
// playback.
//
// pollEvery defaults to 1s when zero.
//
// TODO(user): copy the precise log/abort wording from
// [src/utils/timing-control.ts] when implementing the body.
func HandleTimingControl(ctx context.Context, startTimeUnix int64, pollEvery time.Duration, abort AbortCheck) (int64, error) {
	log := logger.FromContext(ctx)

	if pollEvery <= 0 {
		pollEvery = time.Second
	}

	now := time.Now()
	if startTimeUnix <= 0 {
		log.Debug("timing: start_time not set, joining immediately")
		return now.UnixMilli(), nil
	}

	target := time.Unix(startTimeUnix, 0)
	if !target.After(now) {
		log.Debug("timing: start_time already elapsed", zap.Time("target", target))
		return now.UnixMilli(), nil
	}

	log.Info("timing: waiting for scheduled start",
		zap.Time("target", target),
		zap.Duration("wait", time.Until(target)),
	)

	for time.Now().Before(target) {
		wait := pollEvery
		if remaining := time.Until(target); remaining < wait {
			wait = remaining
		}
		if err := sleep.For(ctx, wait); err != nil {
			return 0, err
		}
		if abort != nil {
			aborted, err := abort(ctx)
			if err != nil {
				return 0, err
			}
			if aborted {
				log.Info("timing: abort fired during scheduled wait")
				return time.Now().UnixMilli(), nil
			}
		}
	}
	return target.UnixMilli(), nil
}
