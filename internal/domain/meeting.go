package domain

import "context"

// MeetingInfo contains parsed information from a meeting URL.
type MeetingInfo struct {
	OriginalURL string
	Provider    string // "google_meet", "microsoft_teams"
	MeetingID   string
	Password    string
}

// MeetingProvider implements provider-specific browser automation logic.
type MeetingProvider interface {
	// Join navigates to the meeting and bypasses the lobby.
	Join(ctx context.Context, page Page, botName string) error

	// Leave cleanly exits the meeting.
	Leave(ctx context.Context, page Page) error
}
