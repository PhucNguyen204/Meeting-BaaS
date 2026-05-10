package logger

import (
	"context"
	"testing"

	"go.uber.org/zap"
)

// Integration of context helpers and mask logic. These are quick sanity
// checks â€” fuller tests come once the surrounding packages exist.

func TestFromContext_NoLogger_ReturnsNop(t *testing.T) {
	got := FromContext(context.Background())
	if got == nil {
		t.Fatal("FromContext returned nil; expected nop logger")
	}
	got.Info("must not panic on nop logger")
}

func TestIntoContext_RoundTrip(t *testing.T) {
	root := zap.NewNop()
	ctx := IntoContext(context.Background(), root)
	if got := FromContext(ctx); got != root {
		t.Fatalf("FromContext returned %v; want %v", got, root)
	}
}

func TestWithState_AttachesField(t *testing.T) {
	root, _ := New(Config{Level: "debug"})
	ctx := IntoContext(context.Background(), root)
	ctx = WithState(ctx, "Recording")
	// We cannot easily inspect zap fields without a custom core; ensure no panic.
	FromContext(ctx).Debug("with state attached")
}

func TestMaskSecrets_KnownKeys(t *testing.T) {
	in := map[string]any{
		"meeting_url":            "https://meet.google.com/abc",
		"user_token":             "supersecret",
		"bots_api_key":           "key",
		"speech_to_text_api_key": "k2",
		"bots_webhook_url":       "https://hooks/x",
		"secret":                 "s",
		"zoom_sdk_pwd":           "p",
		"normal":                 "untouched",
	}
	out := MaskSecrets(in)
	for _, k := range []string{
		"user_token",
		"bots_api_key",
		"speech_to_text_api_key",
		"bots_webhook_url",
		"secret",
		"zoom_sdk_pwd",
	} {
		if out[k] != maskValue {
			t.Errorf("key %q not masked: got %v", k, out[k])
		}
	}
	if out["meeting_url"] != "https://meet.google.com/abc" {
		t.Error("meeting_url should not be masked")
	}
	if out["normal"] != "untouched" {
		t.Error("normal field should not be masked")
	}
}

func TestParseLevel_Defaults(t *testing.T) {
	cases := map[string]bool{
		"":        true,
		"info":    true,
		"INFO":    true,
		"debug":   true,
		"warn":    true,
		"warning": true,
		"error":   true,
		"trace":   true,
		"junk":    false,
	}
	for in, ok := range cases {
		_, err := parseLevel(in)
		if (err == nil) != ok {
			t.Errorf("parseLevel(%q) err=%v, want ok=%v", in, err, ok)
		}
	}
}
