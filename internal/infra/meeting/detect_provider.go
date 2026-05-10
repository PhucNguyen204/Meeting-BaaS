package meeting

import (
	"net/url"
	"strings"

	"github.com/PhucNguyen204/Meeting-BaaS/internal/config"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/pkg/errors"
)

// DetectProvider returns the provider for a given meeting URL.
//
// Port reference: src/utils/detectMeetingProvider.ts.
//
// Recognised hosts:
//   - meet.google.com -> Meet
//   - teams.microsoft.com / teams.live.com -> Teams
//   - zoom.us / zoom.com.cn -> Zoom
//
// Returns ErrInvalidMeetingURL when the URL is empty, malformed, or points
// to an unsupported provider.
func DetectProvider(rawURL string) (config.MeetingProvider, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", errors.ErrInvalidMeetingURL
	}

	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return "", errors.ErrInvalidMeetingURL
	}
	host := strings.ToLower(u.Host)

	switch {
	case strings.Contains(host, "meet.google.com"):
		return config.ProviderMeet, nil
	case strings.Contains(host, "teams.microsoft.com"),
		strings.Contains(host, "teams.live.com"):
		return config.ProviderTeams, nil
	case strings.Contains(host, "zoom.us"),
		strings.Contains(host, "zoom.com.cn"):
		return config.ProviderZoom, nil
	default:
		return "", errors.ErrInvalidMeetingURL
	}
}
