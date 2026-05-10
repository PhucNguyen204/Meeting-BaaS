package config

import (
	"strings"
	"testing"
)

func TestLoadFromBytes_Valid(t *testing.T) {
	raw := `{
		"bot_uuid": "test-uuid",
		"meeting_url": "https://meet.google.com/abc-defg-hij",
		"bot_name": "TestBot",
		"recording_mode": "speaker_view"
	}`
	cfg, err := LoadFromBytes([]byte(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BotUUID != "test-uuid" {
		t.Errorf("got BotUUID %q, want 'test-uuid'", cfg.BotUUID)
	}
	if cfg.MeetingURL != "https://meet.google.com/abc-defg-hij" {
		t.Errorf("got MeetingURL %q", cfg.MeetingURL)
	}
	if cfg.BotName != "TestBot" {
		t.Errorf("got BotName %q", cfg.BotName)
	}
}

func TestLoadFromBytes_Empty(t *testing.T) {
	_, err := LoadFromBytes([]byte(""))
	if err == nil {
		t.Fatal("expected error for empty bytes")
	}
}

func TestLoadFromBytes_InvalidJSON(t *testing.T) {
	_, err := LoadFromBytes([]byte("{invalid"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoadFromBytes_AllFields(t *testing.T) {
	raw := `{
		"id": "id-1",
		"bot_uuid": "uuid-1",
		"user_id": 42,
		"session_id": "session-1",
		"email": "test@example.com",
		"meeting_url": "https://meet.google.com/abc-defg-hij",
		"bot_name": "Bot",
		"enter_message": "Hello",
		"recording_mode": "GalleryView",
		"user_token": "tok",
		"bots_api_key": "key",
		"bots_webhook_url": "https://hooks.example.com",
		"secret": "shhh",
		"streaming_input": "in",
		"streaming_output": "out",
		"streaming_audio_frequency": 16000,
		"custom_branding_bot_path": "/path",
		"automatic_leave": {"waiting_room_timeout": 60, "noone_joined_timeout": 120, "silence_timeout": 30},
		"mp4_s3_path": "s3://bucket/key",
		"environ": "dev",
		"remote": {"api_server_baseurl": "https://api.example.com", "aws_s3_video_bucket": "vids", "aws_s3_log_bucket": "logs"},
		"start_time": 1234567890,
		"exit_time": 9876543210,
		"retry_count": 2,
		"event": {"uuid": "ev-uuid"},
		"extra": {"custom": "data"}
	}`
	cfg, err := LoadFromBytes([]byte(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.UserID != 42 {
		t.Errorf("got UserID %d, want 42", cfg.UserID)
	}
	if cfg.AutomaticLeave.WaitingRoomTimeout != 60 {
		t.Errorf("got WaitingRoomTimeout %d", cfg.AutomaticLeave.WaitingRoomTimeout)
	}
	if cfg.Remote == nil || cfg.Remote.APIServerBaseURL != "https://api.example.com" {
		t.Error("remote not parsed correctly")
	}
	if cfg.Event == nil || cfg.Event.UUID != "ev-uuid" {
		t.Error("event not parsed correctly")
	}
	if cfg.StreamingAudioFrequency != 16000 {
		t.Errorf("got StreamingAudioFrequency %d", cfg.StreamingAudioFrequency)
	}
}

func TestLoadFromFile_NonExistent(t *testing.T) {
	_, err := LoadFromFile("/nonexistent/path/to/config.json")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestLoad_EnvVar(t *testing.T) {
	t.Setenv("BOT_CONFIG_JSON", `{"bot_uuid":"env-uuid","meeting_url":"https://meet.google.com/abc-defg-hij","bot_name":"EnvBot"}`)
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BotUUID != "env-uuid" {
		t.Errorf("got BotUUID %q, want 'env-uuid'", cfg.BotUUID)
	}
}

func TestValidate_Valid(t *testing.T) {
	cfg := &BotConfig{
		BotUUID:    "uuid",
		MeetingURL: "https://meet.google.com/abc-defg-hij",
		BotName:    "Bot",
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.RecordingMode != RecModeSpeakerView {
		t.Errorf("expected default recording mode %q, got %q", RecModeSpeakerView, cfg.RecordingMode)
	}
}

func TestValidate_Nil(t *testing.T) {
	var cfg *BotConfig
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for nil config")
	}
}

func TestValidate_MissingMeetingURL(t *testing.T) {
	cfg := &BotConfig{BotUUID: "uuid", BotName: "Bot"}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing meeting_url")
	}
	if !strings.Contains(err.Error(), "meeting_url") {
		t.Errorf("error should mention meeting_url: %v", err)
	}
}

func TestValidate_MissingBotUUID(t *testing.T) {
	cfg := &BotConfig{MeetingURL: "https://example.com", BotName: "Bot"}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing bot_uuid")
	}
}

func TestValidate_MissingBotName(t *testing.T) {
	cfg := &BotConfig{MeetingURL: "https://example.com", BotUUID: "uuid"}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing bot_name")
	}
}

func TestValidate_NegativeTimeout(t *testing.T) {
	cfg := &BotConfig{
		MeetingURL: "https://example.com",
		BotUUID:    "uuid",
		BotName:    "Bot",
		AutomaticLeave: AutomaticLeave{
			WaitingRoomTimeout: -1,
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for negative timeout")
	}
}

func TestValidate_BadStreamingFrequency(t *testing.T) {
	cfg := &BotConfig{
		MeetingURL:              "https://example.com",
		BotUUID:                 "uuid",
		BotName:                 "Bot",
		StreamingAudioFrequency: 100, // too low
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for bad streaming frequency")
	}
}

func TestValidate_RecordingModeNormalisation(t *testing.T) {
	cases := []struct {
		input RecordingMode
		want  RecordingMode
	}{
		{"SpeakerView", RecModeSpeakerView},
		{"speaker_view", RecModeSpeakerView},
		{"GalleryView", RecModeSpeakerView}, // intentional: gallery → speaker
		{"gallery_view", RecModeSpeakerView},
		{"AudioOnly", RecModeAudioOnly},
		{"audio_only", RecModeAudioOnly},
		{"unknown_mode", "unknown_mode"}, // preserved
	}
	for _, tc := range cases {
		t.Run(string(tc.input), func(t *testing.T) {
			cfg := &BotConfig{
				MeetingURL:    "https://example.com",
				BotUUID:       "uuid",
				BotName:       "Bot",
				RecordingMode: tc.input,
			}
			_ = cfg.Validate()
			if cfg.RecordingMode != tc.want {
				t.Errorf("got %q, want %q", cfg.RecordingMode, tc.want)
			}
		})
	}
}

func TestIsServerless(t *testing.T) {
	var nilCfg *BotConfig
	if !nilCfg.IsServerless() {
		t.Error("nil config should be serverless")
	}
	cfg := &BotConfig{}
	if !cfg.IsServerless() {
		t.Error("config without remote should be serverless")
	}
	cfg.Remote = &RemoteConfig{APIServerBaseURL: "https://api.example.com"}
	if cfg.IsServerless() {
		t.Error("config with remote should NOT be serverless")
	}
}

func TestMaskedClone(t *testing.T) {
	cfg := &BotConfig{
		UserToken:        "secret-token",
		BotsAPIKey:       "secret-key",
		SpeechToTextAPIKey: "stt-key",
		Secret:           "my-secret",
		ZoomSDKPwd:       "zoom-pwd",
		BotUUID:          "visible-uuid",
	}
	clone := cfg.MaskedClone()
	if clone.UserToken != "***MASKED***" {
		t.Errorf("UserToken not masked: %q", clone.UserToken)
	}
	if clone.BotsAPIKey != "***MASKED***" {
		t.Errorf("BotsAPIKey not masked: %q", clone.BotsAPIKey)
	}
	if clone.SpeechToTextAPIKey != "***MASKED***" {
		t.Errorf("SpeechToTextAPIKey not masked: %q", clone.SpeechToTextAPIKey)
	}
	if clone.Secret != "***MASKED***" {
		t.Errorf("Secret not masked: %q", clone.Secret)
	}
	if clone.ZoomSDKPwd != "***MASKED***" {
		t.Errorf("ZoomSDKPwd not masked: %q", clone.ZoomSDKPwd)
	}
	if clone.BotUUID != "visible-uuid" {
		t.Errorf("BotUUID should not be masked: %q", clone.BotUUID)
	}
	// Original should be unmodified.
	if cfg.UserToken != "secret-token" {
		t.Error("original UserToken was modified")
	}
}

func TestMaskedClone_EmptyFields(t *testing.T) {
	cfg := &BotConfig{BotUUID: "uuid"}
	clone := cfg.MaskedClone()
	if clone.UserToken != "" {
		t.Error("empty field should remain empty, not masked")
	}
}
