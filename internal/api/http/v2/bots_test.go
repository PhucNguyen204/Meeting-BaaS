package v2

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// TestValidateCreate exercises the validateCreate helper directly. Pure
// function, no postgres or redis needed.
func TestValidateCreate(t *testing.T) {
	tests := []struct {
		name      string
		req       CreateBotRequest
		scheduled bool
		wantErr   string
	}{
		{
			name:    "missing meeting_url",
			req:     CreateBotRequest{BotName: "X"},
			wantErr: "meeting_url",
		},
		{
			name:    "missing bot_name",
			req:     CreateBotRequest{MeetingURL: "https://meet.google.com/abc"},
			wantErr: "bot_name",
		},
		{
			name:      "scheduled requires join_at",
			req:       CreateBotRequest{MeetingURL: "u", BotName: "X"},
			scheduled: true,
			wantErr:   "join_at",
		},
		{
			name:    "invalid recording_mode",
			req:     CreateBotRequest{MeetingURL: "u", BotName: "X", RecordingMode: "weird"},
			wantErr: "recording_mode",
		},
		{
			name:    "transcription_enabled without provider",
			req:     CreateBotRequest{MeetingURL: "u", BotName: "X", TranscriptionEnabled: true},
			wantErr: "transcription_config",
		},
		{
			name: "valid immediate",
			req:  CreateBotRequest{MeetingURL: "https://meet.google.com/abc", BotName: "X"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCreate(&tc.req, tc.scheduled)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected ok, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

// TestDetectProvider sanity-checks the dispatch.
func TestDetectProvider(t *testing.T) {
	tests := map[string]string{
		"https://meet.google.com/abc-defg-hij": "Meet",
		"https://teams.microsoft.com/l/...":    "Teams",
		"https://us02web.zoom.us/j/12345":      "Zoom",
		"https://example.com/foo":              "unknown",
	}
	for u, want := range tests {
		if got := detectProvider(u); got != want {
			t.Errorf("detectProvider(%q) = %q, want %q", u, got, want)
		}
	}
}

// TestHandleLeaveBot_NoRedis returns 503 when redis is not configured.
func TestHandleLeaveBot_NoRedis(t *testing.T) {
	h := HandleLeaveBot(Deps{})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v2/bots/abc/leave-bot", nil)
	h(rr, req)
	if rr.Code != 503 {
		t.Fatalf("want 503, got %d", rr.Code)
	}
}
