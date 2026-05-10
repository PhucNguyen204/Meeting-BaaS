package states

import (
	"context"

	"github.com/PhucNguyen204/Meeting-BaaS/internal/domain"
	sm "github.com/PhucNguyen204/Meeting-BaaS/internal/usecase/bot"
)

// Recorder abstracts the FFmpeg recording pipeline.
// Implemented by internal/infra/recorder.ScreenRecorder.
type Recorder interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Pause(ctx context.Context) error
	Resume(ctx context.Context) error
	OutputPath() string
}

// Uploader abstracts S3 artifact upload.
// Implemented by internal/infra/storage/s3.Client.
type Uploader interface {
	Upload(ctx context.Context, mc *sm.MeetingContext) error
}

// Webhooker abstracts webhook delivery.
// Implemented by internal/infra/webhook.BotWebhooker.
type Webhooker interface {
	SendComplete(ctx context.Context, mc *sm.MeetingContext) error
	SendError(ctx context.Context, mc *sm.MeetingContext) error
}

// PageHook is a side-effect to install once per page (audio capture binding,
// speakers observer, dialog observer, …). Implemented by:
//   - internal/infra/meeting/meet.AudioCapture (via Enable adapter)
//   - internal/infra/meeting/meet.SpeakersObserver
//   - internal/infra/dialog.Observer
//
// InCallState invokes Attach on every hook after the bot is admitted.
type PageHook interface {
	Attach(ctx context.Context, page domain.Page) error
}

// SpeakerSnapshot is the read-side contract the recording state polls each
// tick to compute attendees count and silence detection. Implemented by
// internal/infra/speaker.Manager.
type SpeakerSnapshot interface {
	// Snapshot returns the currently-active speakers and the union of all
	// names ever seen during the session.
	SnapshotNames() (active []string, allParticipants []string)
}
