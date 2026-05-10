// Package config defines the BotConfig struct that the bot-worker consumes
// at startup. The schema mirrors the original TS [MeetingParams] type
// from [src/types.ts:41-103] so existing bot.config.json files (and the
// upstream API server) can be reused without modification.
//
// Loaders for stdin/file/env live in loader.go; validation lives in
// validator.go.
package config

import "go.uber.org/zap/zapcore"

// MeetingProvider enumerates the supported video conferencing platforms.
//
// Phase 1 only implements Meet; Teams/Zoom are stubs.
type MeetingProvider string

const (
	ProviderMeet  MeetingProvider = "Meet"
	ProviderTeams MeetingProvider = "Teams"
	ProviderZoom  MeetingProvider = "Zoom"
)

// RecordingMode controls the layout requested from the meeting client.
//
// Both PascalCase and snake_case are accepted on input (the upstream
// queue normalises one way, the API normalises the other) — see
// [src/singleton.ts:36-56]. The Validate step in validator.go canonicalises
// to snake_case.
type RecordingMode string

const (
	RecModeSpeakerView RecordingMode = "speaker_view"
	RecModeGalleryView RecordingMode = "gallery_view"
	RecModeAudioOnly   RecordingMode = "audio_only"
)

// SpeechToTextProvider — kept for compatibility, not used in MVP.
type SpeechToTextProvider string

const (
	STTDefault SpeechToTextProvider = "Default"
	STTGladia  SpeechToTextProvider = "Gladia"
	STTRunPod  SpeechToTextProvider = "RunPod"
)

// AutomaticLeave controls when the bot decides the meeting is "over".
//
// Port reference: src/types.ts MeetingParams.automatic_leave.
type AutomaticLeave struct {
	// WaitingRoomTimeout: seconds before giving up on the lobby.
	WaitingRoomTimeout int `json:"waiting_room_timeout" mapstructure:"waiting_room_timeout"`
	// NooneJoinedTimeout: seconds with no other attendee before exiting.
	NooneJoinedTimeout int `json:"noone_joined_timeout" mapstructure:"noone_joined_timeout"`
	// SilenceTimeout: seconds of audio silence before exiting.
	SilenceTimeout int `json:"silence_timeout" mapstructure:"silence_timeout"`
}

// EventInfo carries an external correlation id forwarded to webhooks.
// Mirrors src/types.ts MeetingParams.event.
type EventInfo struct {
	UUID string `json:"uuid" mapstructure:"uuid"`
}

// RemoteConfig identifies the upstream MeetingBaaS-style API server. When
// nil the bot runs in "serverless" mode (no callbacks, no S3 video bucket
// override, no log bucket).
//
// Port reference: src/types.ts MeetingParams.remote.
type RemoteConfig struct {
	APIServerBaseURL  string `json:"api_server_baseurl" mapstructure:"api_server_baseurl"`
	AWSS3VideoBucket  string `json:"aws_s3_video_bucket" mapstructure:"aws_s3_video_bucket"`
	AWSS3LogBucket    string `json:"aws_s3_log_bucket" mapstructure:"aws_s3_log_bucket"`
}

// BotConfig is the canonical bot session descriptor.
//
// Port reference: src/types.ts MeetingParams (entire struct, field-by-field).
//
// Field naming follows the JSON wire format. Go struct tags map snake_case
// JSON to Go fields. Unknown JSON keys are ignored (loader uses encoding/json
// default behaviour).
type BotConfig struct {
	// --- Identity --------------------------------------------------------
	ID        string `json:"id" mapstructure:"id"`
	BotUUID   string `json:"bot_uuid" mapstructure:"bot_uuid"`
	UserID    int64  `json:"user_id" mapstructure:"user_id"`
	SessionID string `json:"session_id" mapstructure:"session_id"`
	Email     string `json:"email" mapstructure:"email"`

	// --- Meeting ---------------------------------------------------------
	MeetingURL      string          `json:"meeting_url" mapstructure:"meeting_url"`
	MeetingProvider MeetingProvider `json:"-" mapstructure:"-"` // set by detector after load
	BotName         string          `json:"bot_name" mapstructure:"bot_name"`
	EnterMessage    string          `json:"enter_message,omitempty" mapstructure:"enter_message"`
	RecordingMode   RecordingMode   `json:"recording_mode" mapstructure:"recording_mode"`

	// --- Auth & API ------------------------------------------------------
	UserToken   string `json:"user_token" mapstructure:"user_token"`
	BotsAPIKey  string `json:"bots_api_key" mapstructure:"bots_api_key"`
	WebhookURL  string `json:"bots_webhook_url,omitempty" mapstructure:"bots_webhook_url"`
	Secret      string `json:"secret,omitempty" mapstructure:"secret"`

	// --- Speech-to-text (compat) -----------------------------------------
	UseMyVocabulary       bool                 `json:"use_my_vocabulary" mapstructure:"use_my_vocabulary"`
	Vocabulary            []string             `json:"vocabulary" mapstructure:"vocabulary"`
	ForceLang             bool                 `json:"force_lang" mapstructure:"force_lang"`
	TranslationLang       string               `json:"translation_lang,omitempty" mapstructure:"translation_lang"`
	SpeechToTextProvider  SpeechToTextProvider `json:"speech_to_text_provider,omitempty" mapstructure:"speech_to_text_provider"`
	SpeechToTextAPIKey    string               `json:"speech_to_text_api_key,omitempty" mapstructure:"speech_to_text_api_key"`

	// --- Streaming -------------------------------------------------------
	StreamingInput          string `json:"streaming_input,omitempty" mapstructure:"streaming_input"`
	StreamingOutput         string `json:"streaming_output,omitempty" mapstructure:"streaming_output"`
	StreamingAudioFrequency int    `json:"streaming_audio_frequency,omitempty" mapstructure:"streaming_audio_frequency"`

	// --- Branding --------------------------------------------------------
	CustomBrandingBotPath string `json:"custom_branding_bot_path,omitempty" mapstructure:"custom_branding_bot_path"`

	// --- Behaviour -------------------------------------------------------
	AutomaticLeave              AutomaticLeave `json:"automatic_leave" mapstructure:"automatic_leave"`
	LocalRecordingServerLocation string        `json:"local_recording_server_location,omitempty" mapstructure:"local_recording_server_location"`

	// --- Output ----------------------------------------------------------
	MP4S3Path string `json:"mp4_s3_path,omitempty" mapstructure:"mp4_s3_path"`

	// --- Environment / infra --------------------------------------------
	Environ                    string        `json:"environ" mapstructure:"environ"`
	AWSS3TemporaryAudioBucket  string        `json:"aws_s3_temporary_audio_bucket,omitempty" mapstructure:"aws_s3_temporary_audio_bucket"`
	Remote                     *RemoteConfig `json:"remote,omitempty" mapstructure:"remote"`

	// --- Timing & retry --------------------------------------------------
	StartTime  int64 `json:"start_time,omitempty" mapstructure:"start_time"`
	ExitTime   int64 `json:"exit_time,omitempty" mapstructure:"exit_time"`
	RetryCount int   `json:"retry_count,omitempty" mapstructure:"retry_count"`

	// --- External correlation --------------------------------------------
	Event *EventInfo     `json:"event,omitempty" mapstructure:"event"`
	Extra map[string]any `json:"extra,omitempty" mapstructure:"extra"`

	// --- Zoom-only (defer phase) -----------------------------------------
	ZoomSDKID  string `json:"zoom_sdk_id,omitempty" mapstructure:"zoom_sdk_id"`
	ZoomSDKPwd string `json:"zoom_sdk_pwd,omitempty" mapstructure:"zoom_sdk_pwd"`
}

// IsServerless reports whether the bot is running without an upstream
// API server (Remote is nil). Mirrors src/singleton.ts isServerless().
func (c *BotConfig) IsServerless() bool {
	return c == nil || c.Remote == nil
}

// MaskedClone returns a copy with sensitive fields redacted, suitable for
// logging the full config at startup. Mirrors the masking block in
// [src/main.ts:217-224].
//
// TODO(user): add zoom_sdk_pwd to the redacted set if Zoom support lands.
func (c *BotConfig) MaskedClone() BotConfig {
	const masked = "***MASKED***"
	out := *c
	if out.UserToken != "" {
		out.UserToken = masked
	}
	if out.BotsAPIKey != "" {
		out.BotsAPIKey = masked
	}
	if out.SpeechToTextAPIKey != "" {
		out.SpeechToTextAPIKey = masked
	}
	if out.ZoomSDKPwd != "" {
		out.ZoomSDKPwd = masked
	}
	if out.Secret != "" {
		out.Secret = masked
	}
	return out
}

// MarshalLogObject implements zapcore.ObjectMarshaler. It automatically
// delegates to a MaskedClone, preventing accidental leakage of secrets
// when the config struct is passed directly to zap.Any or zap.Object.
func (c *BotConfig) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if c == nil {
		return nil
	}
	masked := c.MaskedClone()
	enc.AddString("bot_uuid", masked.BotUUID)
	enc.AddString("meeting_url", masked.MeetingURL)
	enc.AddString("bot_name", masked.BotName)
	enc.AddString("recording_mode", string(masked.RecordingMode))
	enc.AddString("environ", masked.Environ)
	// Add masked sensitive fields
	enc.AddString("user_token", masked.UserToken)
	enc.AddString("bots_api_key", masked.BotsAPIKey)
	enc.AddString("secret", masked.Secret)
	enc.AddString("zoom_sdk_pwd", masked.ZoomSDKPwd)
	return nil
}
