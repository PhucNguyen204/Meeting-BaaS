// Package teams implements the Microsoft Teams [meeting.Provider] for the bot.
//
// Port reference: src/urlParser/teamsUrlParser.ts.
package teams

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/yourorg/meet-bot-go/internal/config"
	"github.com/yourorg/meet-bot-go/internal/logic/meeting"
	perrors "github.com/yourorg/meet-bot-go/internal/pkg/errors"
)

// init registers the parser with the meeting package's URL parser registry.
func init() {
	meeting.Register(config.ProviderTeams, ParseURL)
}

// meetupJoinRegex matches the standard Teams meetup-join URL structure.
//
// Example: https://teams.microsoft.com/l/meetup-join/19:meeting_xxx@thread.v2/0?context=...
var meetupJoinRegex = regexp.MustCompile(
	`https://[a-zA-Z0-9]*\.?teams\.microsoft\.com/l/meetup-join/(.*?)/(\d+)\?context=(.*?)(?:$|&)`,
)

// ParseURL extracts the canonical Teams URL from a user-provided string.
//
// Tolerated input quirks (parity with TS implementation):
//   - Backslash-escaped URL specials (\?, \=, \&) are unescaped.
//   - Google redirect URLs (https://www.google.com/url?q=...) are unwrapped.
//   - Percent-encoded URLs (https%3A...) are decoded.
//   - teams.live.com personal meeting URLs with ?p= password.
//   - teams.microsoft.com standard and launcher URLs.
//   - Already-transformed v2 URLs are passed through.
//
// Port reference: src/urlParser/teamsUrlParser.ts parseMeetingUrlFromJoinInfos().
func ParseURL(_ context.Context, rawURL string) (meeting.MeetingInfo, error) {
	clean := strings.TrimSpace(rawURL)
	if clean == "" {
		return meeting.MeetingInfo{}, fmt.Errorf("Invalid Teams URL: empty: %w", perrors.ErrInvalidMeetingURL)
	}

	// Unescape backslash-escaped URL specials.
	clean = strings.NewReplacer(`\?`, `?`, `\=`, `=`, `\&`, `&`).Replace(clean)

	// Handle Google redirect URLs.
	if strings.HasPrefix(clean, "https://www.google.com/url") {
		u, err := url.Parse(clean)
		if err == nil {
			if q := u.Query().Get("q"); q != "" {
				clean = q
			}
		}
	}

	// Decode percent-encoded full URLs.
	if strings.HasPrefix(clean, "https%3A") {
		decoded, err := url.QueryUnescape(clean)
		if err == nil {
			clean = decoded
		}
	}

	u, err := url.Parse(clean)
	if err != nil {
		return meeting.MeetingInfo{}, fmt.Errorf("Invalid Teams URL: %w", perrors.ErrInvalidMeetingURL)
	}

	host := strings.ToLower(u.Hostname())

	// Handle teams.live.com URLs.
	if strings.Contains(host, "teams.live.com") {
		return parseTeamsLiveURL(u, clean)
	}

	// Handle teams.microsoft.com URLs.
	if strings.Contains(host, "teams.microsoft.com") {
		return parseTeamsMicrosoftURL(u, clean)
	}

	return meeting.MeetingInfo{}, fmt.Errorf("Invalid Teams URL: unsupported host %q: %w", host, perrors.ErrInvalidMeetingURL)
}

// parseTeamsLiveURL handles teams.live.com personal/free meeting URLs.
//
// Formats:
//   - teams.live.com/meet/1234567?p=password
//   - teams.live.com/dl/launcher/launcher.html?url=...
//   - teams.live.com/light-meetings/launch?coords=<base64>
func parseTeamsLiveURL(u *url.URL, rawURL string) (meeting.MeetingInfo, error) {
	// Handle launcher wrapper URLs.
	if strings.HasPrefix(u.Path, "/dl/launcher/") {
		embedded := u.Query().Get("url")
		if embedded != "" {
			meetMatch := regexp.MustCompile(`/meet/(\d+)`).FindStringSubmatch(embedded)
			pMatch := regexp.MustCompile(`[?&]p=([^&]+)`).FindStringSubmatch(embedded)
			if meetMatch != nil {
				meetingCode := meetMatch[1]
				password := ""
				directURL := "https://teams.live.com/meet/" + meetingCode
				if pMatch != nil {
					password = pMatch[1]
					directURL += "?p=" + password + "&anon=true"
				} else {
					directURL += "?anon=true"
				}
				return meeting.MeetingInfo{
					MeetingID: directURL,
					Password:  password,
				}, nil
			}
		}
		return meeting.MeetingInfo{}, fmt.Errorf("Invalid Teams launcher URL: %w", perrors.ErrInvalidMeetingURL)
	}

	// Handle light-meetings URLs.
	if strings.Contains(u.Path, "/light-meetings/launch") {
		return parseLightMeetingsURL(u)
	}

	// Standard teams.live.com/meet/<code>?p=<password> URLs.
	parts := strings.SplitN(u.Path, "/meet/", 2)
	if len(parts) < 2 || parts[1] == "" {
		return meeting.MeetingInfo{}, fmt.Errorf("Invalid Teams live URL format: %w", perrors.ErrInvalidMeetingURL)
	}
	return meeting.MeetingInfo{
		MeetingID: rawURL,
		Password:  u.Query().Get("p"),
	}, nil
}

// parseLightMeetingsURL handles teams.live.com/light-meetings/launch?coords=<base64>.
func parseLightMeetingsURL(u *url.URL) (meeting.MeetingInfo, error) {
	coords := u.Query().Get("coords")
	if coords == "" {
		return meeting.MeetingInfo{}, fmt.Errorf("Invalid Teams light-meetings URL: missing coords: %w", perrors.ErrInvalidMeetingURL)
	}

	decoded, err := base64.StdEncoding.DecodeString(coords)
	if err != nil {
		// Try URL-safe base64.
		decoded, err = base64.URLEncoding.DecodeString(coords)
		if err != nil {
			// Try raw (no padding).
			decoded, err = base64.RawStdEncoding.DecodeString(coords)
			if err != nil {
				return meeting.MeetingInfo{}, fmt.Errorf("Invalid Teams light-meetings URL: bad coords encoding: %w", perrors.ErrInvalidMeetingURL)
			}
		}
	}

	var coordData struct {
		MeetingCode    string `json:"meetingCode"`
		Passcode       string `json:"passcode"`
		ConversationID string `json:"conversationId"`
		TenantID       string `json:"tenantId"`
		MessageID      string `json:"messageId"`
		OrganizerID    string `json:"organizerId"`
	}
	if err := json.Unmarshal(decoded, &coordData); err != nil {
		return meeting.MeetingInfo{}, fmt.Errorf("Invalid Teams light-meetings URL: bad coords JSON: %w", perrors.ErrInvalidMeetingURL)
	}

	// If meetingCode is present, build teams.live.com/meet/... URL.
	if coordData.MeetingCode != "" {
		password := coordData.Passcode
		if password == "" {
			password = u.Query().Get("p")
		}
		directURL := "https://teams.live.com/meet/" + coordData.MeetingCode
		if password != "" {
			directURL += "?p=" + password + "&anon=true"
		} else {
			directURL += "?anon=true"
		}
		return meeting.MeetingInfo{
			MeetingID: directURL,
			Password:  password,
		}, nil
	}

	// If conversationId + tenantId + messageId present, build v2 URL.
	if coordData.ConversationID != "" && coordData.TenantID != "" && coordData.MessageID != "" {
		contextMap := map[string]string{"Tid": coordData.TenantID}
		if coordData.OrganizerID != "" {
			contextMap["Oid"] = coordData.OrganizerID
		}
		ctxJSON, _ := json.Marshal(contextMap)
		directURL := fmt.Sprintf(
			"https://teams.microsoft.com/v2/?meetingjoin=true#/l/meetup-join/%s/%s?context=%s&anon=true",
			coordData.ConversationID, coordData.MessageID, url.QueryEscape(string(ctxJSON)),
		)
		return meeting.MeetingInfo{
			MeetingID: directURL,
			Password:  "",
		}, nil
	}

	return meeting.MeetingInfo{}, fmt.Errorf("Invalid Teams light-meetings URL: insufficient coords data: %w", perrors.ErrInvalidMeetingURL)
}

// parseTeamsMicrosoftURL handles teams.microsoft.com URLs.
func parseTeamsMicrosoftURL(u *url.URL, rawURL string) (meeting.MeetingInfo, error) {
	// Already in v2 format: pass through.
	if strings.Contains(rawURL, "/v2/?meetingjoin=true") {
		return meeting.MeetingInfo{MeetingID: rawURL}, nil
	}

	// Handle light-meetings URLs on teams.microsoft.com.
	if strings.Contains(u.Path, "/light-meetings/launch") {
		return parseLightMeetingsURL(u)
	}

	// Try to transform standard meetup-join URL to v2 format.
	transformed := transformTeamsLink(rawURL)
	return meeting.MeetingInfo{
		MeetingID: transformed,
		Password:  "",
	}, nil
}

// transformTeamsLink converts a standard teams.microsoft.com meetup-join URL
// to the v2 format for better compatibility.
//
// Port reference: src/urlParser/teamsUrlParser.ts transformTeamsLink().
func transformTeamsLink(rawURL string) string {
	m := meetupJoinRegex.FindStringSubmatch(rawURL)
	if m == nil || len(m) < 4 {
		// Cannot transform; append &anon=true and return.
		return appendAnonParam(rawURL)
	}

	threadID := m[1]
	timestamp := m[2]
	ctx := m[3]

	return fmt.Sprintf(
		"https://teams.microsoft.com/v2/?meetingjoin=true#/l/meetup-join/%s/%s?context=%s&anon=true",
		threadID, timestamp, ctx,
	)
}

// appendAnonParam adds &anon=true (or ?anon=true) if not already present.
func appendAnonParam(rawURL string) string {
	if strings.Contains(rawURL, "anon=true") {
		return rawURL
	}
	if strings.Contains(rawURL, "?") {
		return rawURL + "&anon=true"
	}
	return rawURL + "?anon=true"
}
