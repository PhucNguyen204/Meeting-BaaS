package meet

import (
	"context"

	"go.uber.org/zap"

	"github.com/yourorg/meet-bot-go/internal/config"
	"github.com/yourorg/meet-bot-go/internal/logic/browser"
	"github.com/yourorg/meet-bot-go/internal/logic/dialog"
	"github.com/yourorg/meet-bot-go/internal/logic/meeting"
)

// Provider implements [meeting.Provider] for Google Meet.
//
// Port reference: src/meeting/meet.ts (the entire file).
//
// Construction:
//
//	p := meet.NewProvider(log)
//
// The struct is stateless across method calls so a single instance can
// safely serve multiple Pages (one per join attempt).
type Provider struct {
	log      *zap.Logger
	detector *meeting.Detector
}

// NewProvider returns a Meet provider implementation.
func NewProvider(log *zap.Logger) *Provider {
	if log == nil {
		log = zap.NewNop()
	}
	return &Provider{
		log:      log.Named("meet"),
		detector: meeting.NewDetector(MeetStateConfig),
	}
}

// Compile-time check: *Provider satisfies meeting.Provider.
var _ meeting.Provider = (*Provider)(nil)

// Name returns "Meet".
func (p *Provider) Name() config.MeetingProvider { return config.ProviderMeet }

// ParseMeetingURL delegates to ParseURL — kept on the struct so it
// satisfies the Provider interface.
func (p *Provider) ParseMeetingURL(ctx context.Context, meetingURL string) (meeting.MeetingInfo, error) {
	return ParseURL(ctx, meetingURL)
}

// BuildMeetingLink returns the URL the bot should navigate to.
//
// For Google Meet the parsed standard URL already contains everything
// (no per-participant token) so we ignore role/botName/enterMessage.
//
// Port reference: src/meeting/meet.ts buildMeetingLink().
func (p *Provider) BuildMeetingLink(info meeting.MeetingInfo, role int, botName, enterMessage string) string {
	_ = role
	_ = botName
	_ = enterMessage
	return info.MeetingID
}

// FindEndMeeting reports whether the bot has been removed from the call.
//
// Implementation should evaluate MeetStateConfig and return true on
// "removed" or "meeting_ended" states.
//
// Port reference: src/meeting/meet.ts findEndMeeting().
//
// TODO(user): use the detector once Detect() is implemented.
func (p *Provider) FindEndMeeting(ctx context.Context, page browser.Page) (bool, error) {
	state, err := p.detector.Detect(ctx, page)
	if err != nil {
		return false, err
	}
	return state == "removed", nil
}

// (other Provider methods are split into separate files for readability:
// open_page.go, join.go, close.go.)

// dialogObserver is a small adaptor to keep the JoinOptions DialogObserver
// type local to this package.
type dialogObserver = dialog.Observer
