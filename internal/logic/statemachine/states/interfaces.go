package states

import (
	"context"

	sm "github.com/yourorg/meet-bot-go/internal/logic/statemachine"
)

// Recorder abstracts the FFmpeg recording pipeline.
// Implemented by internal/logic/recorder.ScreenRecorder.
type Recorder interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Pause(ctx context.Context) error
	Resume(ctx context.Context) error
	OutputPath() string
}

// Uploader abstracts S3 artifact upload.
// Implemented by internal/dataaccess/s3.Client.
type Uploader interface {
	Upload(ctx context.Context, mc *sm.MeetingContext) error
}

// Webhooker abstracts webhook delivery.
// Implemented by internal/logic/webhook.Sender.
type Webhooker interface {
	SendComplete(ctx context.Context, mc *sm.MeetingContext) error
	SendError(ctx context.Context, mc *sm.MeetingContext) error
}
