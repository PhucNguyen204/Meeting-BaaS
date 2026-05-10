package meet

import (
	"time"

	"github.com/yourorg/meet-bot-go/internal/logic/meeting"
)

// MeetStateConfig is the StateConfig used by the generic state detector
// to track Google Meet's pre-call / in-call UI states.
//
// Port reference: src/meeting/meet-state-config.ts.
//
// TODO(user): Keep selectors aligned with [selectors.go]. The TS source
// includes a few additional rules we haven't ported yet (e.g. captcha-like
// "verify it's you" prompt) — add them here when porting in_call_state.
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
