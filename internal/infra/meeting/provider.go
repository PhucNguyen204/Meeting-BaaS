// Package meeting defines the provider-agnostic interface every video
// conferencing platform implementation (Meet, Teams, Zoom) must satisfy.
//
// Phase 1 only ships the Meet implementation under ./meet ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â‚¬Å¡Ã‚Â¬Ãƒâ€šÃ‚Â Teams/Zoom
// remain placeholders until later phases.
//
// Port reference: src/types.ts MeetingProviderInterface.
package meeting

import (
	"context"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/domain"

	"github.com/PhucNguyen204/Meeting-BaaS/internal/config"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/infra/dialog"
)

// MeetingInfo is the parsed result of a meeting URL.
//
// Port reference: src/types.ts MeetingProviderInterface.parseMeetingUrl().
type MeetingInfo struct {
	MeetingID string
	Password  string
}

// JoinOptions controls how the provider joins the meeting.
//
// Used by the WaitingRoom state to coordinate timing, abort, and the
// post-join callback.
type JoinOptions struct {
	BotName        string
	EnterMessage   string
	StartTimeUnix  int64
	DialogObserver *dialog.Observer
	OnJoinSuccess  func()
	CancelCheck    func() bool
}

// Provider is the contract every meeting platform implementation must
// satisfy. Methods are 1-1 with [src/types.ts MeetingProviderInterface].
//
// Implementations live under sub-packages: meeting/meet, meeting/teams (later).
type Provider interface {
	// Name returns the provider identifier ("Meet" / "Teams" / "Zoom").
	Name() config.MeetingProvider

	// ParseMeetingURL extracts the meeting id (and password if any) from
	// the user-provided meeting URL.
	ParseMeetingURL(ctx context.Context, meetingURL string) (MeetingInfo, error)

	// BuildMeetingLink converts the parsed identifiers back to the URL
	// the bot should navigate to. May differ from the input URL (Teams
	// rewrites; Zoom adds query params; Meet returns as-is).
	BuildMeetingLink(info MeetingInfo, role int, botName, enterMessage string) string

	// OpenMeetingPage navigates the browser to the meeting URL and prepares
	// the page for the join flow (audio capture init, page checks, etc).
	OpenMeetingPage(ctx context.Context, bctx domain.BrowserContext, link, streamingInput string) (domain.Page, error)

	// JoinMeeting walks the bot through the lobby/Ask-to-join flow and
	// blocks until either accepted or rejected.
	JoinMeeting(ctx context.Context, page domain.Page, opts JoinOptions) error

	// FindEndMeeting reports whether the bot has been removed from the
	// active call (host kicked, "you've been removed" UI, etc).
	FindEndMeeting(ctx context.Context, page domain.Page) (bool, error)

	// CloseMeeting tries to gracefully leave the call.
	CloseMeeting(ctx context.Context, page domain.Page) error
}
