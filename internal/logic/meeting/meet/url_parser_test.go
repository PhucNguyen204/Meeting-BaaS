package meet

import (
	"context"
	"errors"
	"testing"

	perrors "github.com/yourorg/meet-bot-go/internal/pkg/errors"
)

// Port reference: src/urlParser/meetUrlParser.test.ts.

func TestParseURL_Valid(t *testing.T) {
	cases := []struct {
		name string
		in   string
		out  string
	}{
		{
			name: "standard Meet URL",
			in:   "https://meet.google.com/abc-defg-hij",
			out:  "https://meet.google.com/abc-defg-hij",
		},
		{
			name: "Meet URL with query parameters",
			in:   "https://meet.google.com/abc-defg-hij?authuser=0",
			out:  "https://meet.google.com/abc-defg-hij?authuser=0",
		},
		{
			name: "Meet URL without https",
			in:   "meet.google.com/abc-defg-hij",
			out:  "https://meet.google.com/abc-defg-hij",
		},
		{
			name: "Meet URL with multiple query parameters",
			in:   "https://meet.google.com/abc-defg-hij?authuser=0&hs=178",
			out:  "https://meet.google.com/abc-defg-hij?authuser=0&hs=178",
		},
		{
			name: "Meet URL with www subdomain",
			in:   "https://www.meet.google.com/abc-defg-hij",
			out:  "https://meet.google.com/abc-defg-hij",
		},
		{
			name: "Meet URL with encoded characters in query",
			in:   "https://meet.google.com/abc-defg-hij?authuser=test%40gmail.com",
			out:  "https://meet.google.com/abc-defg-hij?authuser=test%40gmail.com",
		},
		{
			name: "Meet URL with accidental prefix",
			in:   "jhttps://meet.google.com/abc-defg-hij",
			out:  "https://meet.google.com/abc-defg-hij",
		},
		{
			name: "Meet URL with quotes",
			in:   `"https://meet.google.com/abc-defg-hij"`,
			out:  "https://meet.google.com/abc-defg-hij",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info, err := ParseURL(context.Background(), tc.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if info.MeetingID != tc.out {
				t.Errorf("got %q want %q", info.MeetingID, tc.out)
			}
			if info.Password != "" {
				t.Errorf("got password %q, want empty", info.Password)
			}
		})
	}
}

func TestParseURL_Invalid(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"empty URL", ""},
		{"wrong domain", "https://google.com/abc-defg-hij"},
		{"invalid code format", "https://meet.google.com/abcd-efgh-ijkl"},
		{"missing code parts", "https://meet.google.com/abc-defg"},
		{"invalid characters in code", "https://meet.google.com/123-4567-890"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseURL(context.Background(), tc.in)
			if err == nil {
				t.Fatalf("expected error for %q, got nil", tc.in)
			}
			if !errors.Is(err, perrors.ErrInvalidMeetingURL) {
				t.Errorf("err %v not wrapping ErrInvalidMeetingURL", err)
			}
		})
	}
}
