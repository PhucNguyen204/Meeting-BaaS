package domain

import "context"

// Speaker represents an active participant speaking in the meeting.
type Speaker struct {
	Name      string
	IsActive  bool
	Timestamp int64
}

// SpeakerManager tracks and emits active speaker events.
type SpeakerManager interface {
	HandleActiveSpeaker(ctx context.Context, speaker Speaker) error
	Stop()
}
