package teams

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	perrors "github.com/yourorg/meet-bot-go/internal/pkg/errors"
)

// Port reference: src/urlParser/teamsUrlParser.test.ts

func TestParseURL_StandardMicrosoftURLs(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{
			name: "standard meetup-join with context",
			in:   "https://teams.microsoft.com/l/meetup-join/19%3ameeting_MjM0OTEwZmEtMGU1Yi00MjA4LTgwNmUtZDUzYWY3YWE2MmZj%40thread.v2/0?context=%7b%22Tid%22%3a%228dd08955-18a8-4cd7-8017-5f997f4d47af%22%2c%22Oid%22%3a%220fab73dc-0c6c-4780-9032-1c19b5a545c3%22%7d",
		},
		{
			name: "standard meetup-join with partial context",
			in:   "https://teams.microsoft.com/l/meetup-join/19%3ameeting_OWIwY2ZhYzQtMGVjMC00ZTE4LTgwMzctMDU0MzBmMzg2ZDJl%40thread.v2/0?context=%7b%22Tid%22%3a%228dd08955-18a8-4cd7-8017-5f997f4d47af%22%7d",
		},
		{
			name: "meetup-join unencoded @",
			in:   "https://teams.microsoft.com/l/meetup-join/19:meeting_MDYyNDgzMmQtODg2Ni00MjBmLTk4YTAtZjYwMTQ0MGNiMmNl@thread.v2/0?context=%7B%22Tid%22:%222dbdd394-741d-4914-9993-ea4584a95749%22%7D",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info, err := ParseURL(context.Background(), tc.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if info.MeetingID == "" {
				t.Error("expected non-empty MeetingID")
			}
			if info.Password != "" {
				t.Errorf("expected empty password, got %q", info.Password)
			}
		})
	}
}

func TestParseURL_TeamsLiveURLs(t *testing.T) {
	cases := []struct {
		name         string
		in           string
		wantPassword string
	}{
		{
			name:         "live URL with password",
			in:           "https://teams.live.com/meet/9356969621606?p=08ogAWeCL73fVssuEK",
			wantPassword: "08ogAWeCL73fVssuEK",
		},
		{
			name:         "live URL with different password",
			in:           "https://teams.live.com/meet/9339528342593?p=VGZGxvTVLIyZ81WauE",
			wantPassword: "VGZGxvTVLIyZ81WauE",
		},
		{
			name:         "live URL with password 3",
			in:           "https://teams.live.com/meet/9314184555833?p=00ewkGrA1OJD7Id1NR",
			wantPassword: "00ewkGrA1OJD7Id1NR",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info, err := ParseURL(context.Background(), tc.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if info.MeetingID != tc.in {
				t.Errorf("got meeting ID %q, want %q", info.MeetingID, tc.in)
			}
			if info.Password != tc.wantPassword {
				t.Errorf("got password %q, want %q", info.Password, tc.wantPassword)
			}
		})
	}
}

func TestParseURL_TACV2URLs(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{
			name: "tacv2 URL",
			in:   "https://teams.microsoft.com/l/meetup-join/19:alTrvfJlXitdMLLxjio8rfnHDhKWaZ3_M-EwK5ewWHg1@thread.tacv2/1730831739131?context=%7B%22Tid%22:%221eba988e-f725-4323-976e-38aaba6ee3a3%22%7D",
		},
		{
			name: "tacv2 URL with Oid",
			in:   "https://teams.microsoft.com/l/meetup-join/19:alTrvfJlXitdMLLxjio8rfnHDhKWaZ3_M-EwK5ewWHg1@thread.tacv2/1731342990116?context=%7BTid:1eba988e-f725-4323-976e-38aaba6ee3a3,Oid:2f8f4d50-3e1b-41ea-99fe-4361ba60ada5%7D",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info, err := ParseURL(context.Background(), tc.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if info.MeetingID == "" {
				t.Error("expected non-empty MeetingID")
			}
			if info.Password != "" {
				t.Errorf("expected empty password, got %q", info.Password)
			}
		})
	}
}

func TestParseURL_SubdomainURLs(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{
			name: "us02web subdomain",
			in:   "https://us02web.teams.microsoft.com/l/meetup-join/19:meeting_123@thread.v2/0",
		},
		{
			name: "us06web subdomain",
			in:   "https://us06web.teams.microsoft.com/l/meetup-join/19:meeting_456@thread.v2/0",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info, err := ParseURL(context.Background(), tc.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if info.MeetingID == "" {
				t.Error("expected non-empty MeetingID")
			}
		})
	}
}

func TestParseURL_V2Passthrough(t *testing.T) {
	input := "https://teams.microsoft.com/v2/?meetingjoin=true#/l/meetup-join/19:meeting@thread.v2/0?context=abc&anon=true"
	info, err := ParseURL(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.MeetingID != input {
		t.Errorf("v2 URL should pass through unchanged, got %q", info.MeetingID)
	}
}

func TestParseURL_GoogleRedirectURLs(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{
			name: "google redirect wrapping teams",
			in:   "https://www.google.com/url?q=https://teams.microsoft.com/l/meetup-join/19%3ameeting_OTUzODNjNmEtNjIwMC00MzkxLWExYjktNWMyMDY2NTE3Yzhk%40thread.v2/0?context=%7B%22Tid%22:%222dbdd394%22%7D",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info, err := ParseURL(context.Background(), tc.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if info.MeetingID == "" {
				t.Error("expected non-empty MeetingID")
			}
		})
	}
}

func TestParseURL_EncodedURL(t *testing.T) {
	original := "https://teams.microsoft.com/l/meetup-join/19:meeting_456@thread.v2/0?context=test"
	encoded := "https%3A%2F%2Fteams.microsoft.com%2Fl%2Fmeetup-join%2F19%3Ameeting_456%40thread.v2%2F0%3Fcontext%3Dtest"
	info, err := ParseURL(context.Background(), encoded)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = original
	if info.MeetingID == "" {
		t.Error("expected non-empty MeetingID from encoded URL")
	}
}

func TestParseURL_BackslashEscaped(t *testing.T) {
	input := `https://teams.live.com/meet/9356969621606\?p\=testpwd`
	info, err := ParseURL(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Password != "testpwd" {
		t.Errorf("got password %q, want 'testpwd'", info.Password)
	}
}

func TestParseURL_Invalid(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"empty URL", ""},
		{"not teams domain", "https://not-teams.com/meeting"},
		{"zoom URL on teams domain name", "https://teams.zoom.us/j/123456"},
		{"not a URL", "not-a-url"},
		{"teams.com without valid format", "https://teams.com/invalid-format"},
		{"whitespace only", "   "},
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

func TestParseURL_TeamsLiveLauncherURL(t *testing.T) {
	// Simulate launcher URL with embedded meet path.
	// Real launcher URLs use percent-encoded fragment (#→%23).
	input := "https://teams.live.com/dl/launcher/launcher.html?url=%2F_%23%2Fmeet%2F123456%3Fp%3Dabc%26anon%3Dtrue"
	info, err := ParseURL(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Password != "abc" {
		t.Errorf("got password %q, want 'abc'", info.Password)
	}
	if info.MeetingID == "" {
		t.Error("expected non-empty MeetingID")
	}
}

func TestParseURL_TeamsLiveLauncherURL_NoMeetPath(t *testing.T) {
	input := "https://teams.live.com/dl/launcher/launcher.html?url=/_#/other/path"
	_, err := ParseURL(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for launcher URL without meet path")
	}
}

func TestParseURL_TeamsLiveInvalidPath(t *testing.T) {
	input := "https://teams.live.com/invalid/path"
	_, err := ParseURL(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for invalid teams.live.com path")
	}
}

func TestParseURL_LightMeetingsWithMeetingCode(t *testing.T) {
	coordData := `{"meetingCode":"12345","passcode":"secret123"}`
	encoded := base64.StdEncoding.EncodeToString([]byte(coordData))
	input := "https://teams.live.com/light-meetings/launch?coords=" + encoded
	info, err := ParseURL(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Password != "secret123" {
		t.Errorf("got password %q, want 'secret123'", info.Password)
	}
	if info.MeetingID == "" {
		t.Error("expected non-empty MeetingID")
	}
}

func TestParseURL_LightMeetingsWithConversation(t *testing.T) {
	coordData := `{"conversationId":"19:abc@thread.v2","tenantId":"tenant-123","messageId":"1234567890"}`
	encoded := base64.StdEncoding.EncodeToString([]byte(coordData))
	input := "https://teams.microsoft.com/light-meetings/launch?coords=" + encoded
	info, err := ParseURL(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.MeetingID == "" {
		t.Error("expected non-empty MeetingID")
	}
	if info.Password != "" {
		t.Errorf("expected empty password, got %q", info.Password)
	}
}

func TestParseURL_LightMeetingsMissingCoords(t *testing.T) {
	input := "https://teams.live.com/light-meetings/launch"
	_, err := ParseURL(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for light-meetings without coords")
	}
}

func TestParseURL_LightMeetingsBadBase64(t *testing.T) {
	input := "https://teams.live.com/light-meetings/launch?coords=!!!not-base64!!!"
	_, err := ParseURL(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for bad base64 coords")
	}
}

func TestParseURL_LightMeetingsBadJSON(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("not json"))
	input := "https://teams.live.com/light-meetings/launch?coords=" + encoded
	_, err := ParseURL(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for bad JSON in coords")
	}
}

func TestParseURL_LightMeetingsInsufficientData(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte(`{"other":"field"}`))
	input := "https://teams.live.com/light-meetings/launch?coords=" + encoded
	_, err := ParseURL(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for insufficient coords data")
	}
}

func TestAppendAnonParam(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"no params", "https://example.com", "https://example.com?anon=true"},
		{"has params", "https://example.com?x=1", "https://example.com?x=1&anon=true"},
		{"already has anon", "https://example.com?anon=true", "https://example.com?anon=true"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := appendAnonParam(tc.in)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
