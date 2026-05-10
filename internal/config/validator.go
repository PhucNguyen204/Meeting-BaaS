package config

import (
	"fmt"
	"strings"
)

// Validate checks that mandatory fields are present and normalises a few
// values to canonical form (e.g. recording mode aliases).
//
// Mirrors the constructor checks in [src/singleton.ts:58-86] and the
// initialisation guard in [src/state-machine/states/initialization-state.ts:18-21].
//
// Returns the first validation error encountered. Successful return means
// the bot can safely begin its initialisation flow.
//
// TODO(user): convert to multi-error reporting once we wire a validator
// library — the original TS only checks one field at a time, this is OK
// for now.
func (c *BotConfig) Validate() error {
	if c == nil {
		return fmt.Errorf("config: nil BotConfig")
	}
	if strings.TrimSpace(c.MeetingURL) == "" {
		return fmt.Errorf("config: meeting_url is required")
	}
	if strings.TrimSpace(c.BotUUID) == "" {
		return fmt.Errorf("config: bot_uuid is required")
	}
	if strings.TrimSpace(c.BotName) == "" {
		return fmt.Errorf("config: bot_name is required")
	}
	if c.RecordingMode == "" {
		c.RecordingMode = RecModeSpeakerView
	} else {
		c.RecordingMode = normaliseRecordingMode(c.RecordingMode)
	}
	if c.AutomaticLeave.WaitingRoomTimeout < 0 ||
		c.AutomaticLeave.NooneJoinedTimeout < 0 ||
		c.AutomaticLeave.SilenceTimeout < 0 {
		return fmt.Errorf("config: automatic_leave timeouts must be >= 0")
	}
	if c.StreamingAudioFrequency != 0 &&
		(c.StreamingAudioFrequency < 8000 || c.StreamingAudioFrequency > 96000) {
		return fmt.Errorf("config: streaming_audio_frequency out of range: %d", c.StreamingAudioFrequency)
	}
	return nil
}

// normaliseRecordingMode collapses PascalCase aliases to snake_case.
//
// Port reference: src/singleton.ts:36-56 normalizeRecordingMode().
//
// Note: gallery_view currently maps to speaker_view because the recorder
// pipeline does not yet honour gallery layouts (parity with TS behaviour).
func normaliseRecordingMode(m RecordingMode) RecordingMode {
	switch m {
	case "speaker_view", "SpeakerView":
		return RecModeSpeakerView
	case "gallery_view", "GalleryView":
		// Intentional: TS maps gallery to speaker. Revisit when implementing
		// gallery layout in the recorder.
		return RecModeSpeakerView
	case "audio_only", "AudioOnly":
		return RecModeAudioOnly
	default:
		// Preserve the original value so Validate's caller sees the bad input.
		return m
	}
}
