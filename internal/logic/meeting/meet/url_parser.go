// Package meet implements the Google Meet [meeting.Provider] for the bot.
//
// File-by-file port reference:
//
//	provider.go            <- src/meeting/meet.ts (struct + interface methods)
//	url_parser.go          <- src/urlParser/meetUrlParser.ts
//	url_parser_test.go     <- src/urlParser/meetUrlParser.test.ts
//	open_page.go           <- src/meeting/meet.ts openMeetingPage()
//	join.go                <- src/meeting/meet.ts joinMeeting()
//	close.go               <- src/meeting/meet/closeMeeting.ts
//	audio_capture.go       <- src/meeting/meet/audio-capture.ts
//	html_cleaner.go        <- src/meeting/meet/htmlCleaner.ts
//	speakers_observer.go   <- src/meeting/meet/speakersObserver.ts
//	send_message.go        <- src/meeting/meet/sendEntryMessage.ts
//	state_config.go        <- src/meeting/meet-state-config.ts
//	selectors.go           <- (consolidated from various TS files)
package meet

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/yourorg/meet-bot-go/internal/config"
	"github.com/yourorg/meet-bot-go/internal/logic/meeting"
	perrors "github.com/yourorg/meet-bot-go/internal/pkg/errors"
)

// init registers the parser with the meeting package's URL parser registry.
func init() {
	meeting.Register(config.ProviderMeet, ParseURL)
}

// meetCodeRegex matches the canonical Meet URL shape:
//
//	meet.google.com/<3-letters>-<4-letters>-<3-letters>[?query]
//
// Port reference: src/urlParser/meetUrlParser.ts:23-24.
var meetCodeRegex = regexp.MustCompile(`meet\.google\.com\/([a-z]{3}-[a-z]{4}-[a-z]{3})((?:\?.*)?$)`)

// ParseURL extracts the canonical Meet URL from a user-provided string.
//
// Tolerated input quirks (parity with TS implementation):
//   - Surrounding double quotes are stripped.
//   - Backslash-escaped query chars (\?, \=, \&) are unescaped.
//   - "meet.google.com/..." without scheme is accepted.
//   - URLs may be embedded in surrounding text — the first whitespace-separated
//     token containing "meet.google.com" wins.
//   - "www.meet.google.com" is collapsed to "meet.google.com".
//
// Rejects:
//   - Empty input.
//   - Domains other than meet.google.com.
//   - Meet codes not matching the [a-z]{3}-[a-z]{4}-[a-z]{3} pattern.
//
// Port reference: src/urlParser/meetUrlParser.ts.
func ParseURL(ctx context.Context, rawURL string) (meeting.MeetingInfo, error) {
	_ = ctx

	clean := strings.TrimSpace(rawURL)
	if clean == "" {
		return meeting.MeetingInfo{}, fmt.Errorf("Invalid Google Meet URL format: empty: %w", perrors.ErrInvalidMeetingURL)
	}
	// Strip surrounding double quotes.
	if len(clean) >= 2 && clean[0] == '"' && clean[len(clean)-1] == '"' {
		clean = clean[1 : len(clean)-1]
	}
	// Unescape backslash-escaped URL specials.
	clean = strings.NewReplacer(`\?`, `?`, `\=`, `=`, `\&`, `&`).Replace(clean)
	// Bare "meet.google.com/..." gets a scheme prefix.
	if strings.HasPrefix(clean, "meet.") {
		clean = "https://" + clean
	}

	// Find the first whitespace token containing meet.google.com.
	var candidate string
	for _, tok := range strings.Fields(clean) {
		if strings.Contains(tok, "meet.google.com") {
			candidate = tok
			break
		}
	}
	if candidate == "" {
		return meeting.MeetingInfo{}, fmt.Errorf("Invalid Google Meet URL format: no meet.google.com host: %w", perrors.ErrInvalidMeetingURL)
	}

	m := meetCodeRegex.FindStringSubmatch(candidate)
	if m == nil {
		return meeting.MeetingInfo{}, fmt.Errorf("Invalid Google Meet URL format: %w", perrors.ErrInvalidMeetingURL)
	}
	meetCode := m[1]
	query := m[2]

	standard := "https://meet.google.com/" + meetCode + query
	return meeting.MeetingInfo{
		MeetingID: standard,
		Password:  "",
	}, nil
}
