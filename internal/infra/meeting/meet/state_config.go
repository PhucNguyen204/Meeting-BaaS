package meet

import (
	"time"

	"github.com/PhucNguyen204/Meeting-BaaS/internal/infra/meeting"
)

// MeetStateConfig is the StateConfig used by the generic state detector
// to track Google Meet's pre-call / in-call UI states.
//
// Port reference: src/meeting/meet-state-config.ts.
//
// TODO(user): Keep selectors aligned with [selectors.go]. The TS source
// includes a few additional rules we haven't ported yet (e.g. captcha-like
// "verify it's you" prompt) ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â‚¬Å¡Ã‚Â¬Ãƒâ€¦Ã‚Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â add them here when porting in_call_state.
var MeetStateConfig = meeting.StateConfig{
	Provider: "Meet",
	States: []meeting.StateRule{
		{
			Name:            "removed",
			AnyOf:           []string{SelRemovedFromCallText, SelMeetingEndedText},
			EvaluateTimeout: time.Second,
		},
		{
			Name:            "login_required",
			AnyOf:           []string{SelLoginRequiredText},
			EvaluateTimeout: time.Second,
		},
		{
			Name:            "lobby_denied",
			AnyOf:           []string{SelLobbyDeniedText},
			EvaluateTimeout: time.Second,
		},
		{
			Name:            "in_call",
			AnyOf:           []string{SelLeaveCallButton},
			EvaluateTimeout: time.Second,
		},
		{
			Name:            "lobby_open",
			AnyOf:           []string{SelAskToJoinButton, SelAskToJoinButtonText, SelJoinNowButton},
			EvaluateTimeout: time.Second,
		},
	},
}
