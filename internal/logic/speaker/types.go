// Package speaker tracks who is speaking inside the meeting and feeds
// that information to the recorder (overlays, layout) and the streaming
// pipeline (per-speaker channels).
//
// Phase 1 ships only the data types and a passthrough Manager skeleton.
// The full implementation (debouncing, name normalisation, S3 export of
// the speaker timeline) lands in Phase 5.
//
// Port reference:
//   - src/meeting/speakersObserver.ts (DOM-side data shape)
//   - src/state-machine/states/recording-state.ts (consumer side)
package speaker

import "time"

// SpeakerEvent represents a speaker becoming active or inactive at a
// specific moment.
type SpeakerEvent struct {
	// Name as displayed by the meeting client.
	Name string

	// ParticipantID is provider-specific (Meet allocation index, Teams
	// participant id, ...). Unique within a session.
	ParticipantID string

	// Active reports whether the participant is currently producing audio.
	Active bool

	// At is the wallclock instant in the bot's timeline.
	At time.Time

	// AtMillisFromStart is At - sessionStart in milliseconds. This is the
	// timeline anchor used by the recorder so playback offsets line up
	// regardless of NTP drift.
	AtMillisFromStart int64
}

// Snapshot is the periodically-emitted summary of who has spoken.
//
// Used by the recorder to overlay names; used by the streaming pipeline
// to switch active audio channel.
type Snapshot struct {
	At              time.Time
	ActiveSpeakers  []string
	AllParticipants []string
}
