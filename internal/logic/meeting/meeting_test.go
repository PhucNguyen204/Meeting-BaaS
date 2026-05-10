package meeting

import (
	"context"
	"errors"
	"testing"

	"github.com/yourorg/meet-bot-go/internal/config"
	perrors "github.com/yourorg/meet-bot-go/internal/pkg/errors"
)

func TestDetectProvider(t *testing.T) {
	cases := []struct {
		name     string
		url      string
		want     config.MeetingProvider
		wantErr  bool
	}{
		{"meet", "https://meet.google.com/abc-defg-hij", config.ProviderMeet, false},
		{"teams", "https://teams.microsoft.com/l/meetup-join/19:meeting@thread.v2/0", config.ProviderTeams, false},
		{"teams live", "https://teams.live.com/meet/12345", config.ProviderTeams, false},
		{"zoom us", "https://zoom.us/j/12345", config.ProviderZoom, false},
		{"zoom cn", "https://zoom.com.cn/j/12345", config.ProviderZoom, false},
		{"empty", "", "", true},
		{"no host", "just-text", "", true},
		{"unsupported", "https://example.com/meeting", "", true},
		{"whitespace only", "   ", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DetectProvider(tc.url)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tc.url)
				}
				if !errors.Is(err, perrors.ErrInvalidMeetingURL) {
					t.Errorf("error should wrap ErrInvalidMeetingURL: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseURL_WithRegisteredParser(t *testing.T) {
	// Register a fake parser for testing.
	Register(config.ProviderZoom, func(_ context.Context, rawURL string) (MeetingInfo, error) {
		return MeetingInfo{MeetingID: "parsed:" + rawURL}, nil
	})
	defer func() { delete(parsers, config.ProviderZoom) }()

	prov, info, err := ParseURL(context.Background(), "https://zoom.us/j/12345")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prov != config.ProviderZoom {
		t.Errorf("got provider %q, want Zoom", prov)
	}
	if info.MeetingID != "parsed:https://zoom.us/j/12345" {
		t.Errorf("got %q", info.MeetingID)
	}
}

func TestParseURL_NoParserRegistered(t *testing.T) {
	// Zoom has no parser by default (unless imported).
	delete(parsers, config.ProviderZoom)
	prov, _, err := ParseURL(context.Background(), "https://zoom.us/j/12345")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prov != config.ProviderZoom {
		t.Errorf("got provider %q", prov)
	}
}

func TestParseURL_Invalid(t *testing.T) {
	_, _, err := ParseURL(context.Background(), "https://example.com/meeting")
	if err == nil {
		t.Fatal("expected error for unsupported domain")
	}
}

func TestNewDetector(t *testing.T) {
	cfg := StateConfig{Provider: "test", States: []StateRule{}}
	d := NewDetector(cfg)
	if d == nil {
		t.Fatal("expected non-nil detector")
	}
}

func TestDetector_Detect_NilPage(t *testing.T) {
	d := NewDetector(StateConfig{})
	state, err := d.Detect(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state != "" {
		t.Errorf("expected empty state for nil page, got %q", state)
	}
}

func TestDetector_Detect_NilDetector(t *testing.T) {
	var d *Detector
	state, err := d.Detect(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state != "" {
		t.Errorf("expected empty state for nil detector, got %q", state)
	}
}
