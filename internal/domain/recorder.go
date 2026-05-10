package domain

import "context"

// Recorder manages an FFmpeg screen and audio recording process.
type Recorder interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Pause(ctx context.Context) error
	Resume(ctx context.Context) error
	OutputPath() string
}
