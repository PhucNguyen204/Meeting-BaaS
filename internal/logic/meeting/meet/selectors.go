package meet

// CSS selectors used throughout the Meet flow. Centralising them in one
// file makes it easy to fix UI breakage in a single PR when Google Meet
// changes its DOM (which happens roughly quarterly).
//
// Port reference: scattered across src/meeting/meet.ts, src/meeting/meet/*.ts.
//
// TODO(user): keep these in sync with the production UI; failing selectors
// surface as join_failures in the bot logs.
const (
	// --- Pre-call screen --------------------------------------------------
	SelNameInput            = `input[type="text"][aria-label="Your name"]`
	SelNameInputFallback    = `input[placeholder*="name"]`
	SelAskToJoinButton      = `button[jsname="Qx7uuf"]`
	SelAskToJoinButtonText  = `text=Ask to join`
	SelJoinNowButton        = `button[jsname="A5il2e"]`
	SelDismissPrejoinTooltip = `[role="dialog"] button[aria-label="Close"]`

	// --- In-call UI -------------------------------------------------------
	SelLeaveCallButton  = `button[aria-label="Leave call"]`
	SelMicToggleButton  = `button[aria-label*="microphone"]`
	SelCamToggleButton  = `button[aria-label*="camera"]`
	SelParticipantsList = `[aria-label="Participants"]`
	SelChatPanel        = `[aria-label="Chat with everyone"]`
	SelChatToggleButton = `button[aria-label="Chat with everyone"]`
	SelChatTextarea     = `textarea[aria-label="Send a message to everyone"]`
	SelChatSendButton   = `button[aria-label="Send a message to everyone"]`

	// --- "Removed from meeting" / errors ---------------------------------
	SelRemovedFromCallText  = `text=You've been removed from the meeting`
	SelMeetingEndedText     = `text=This meeting has ended`
	SelLoginRequiredText    = `text=Sign in to your Google Account`
	SelLobbyDeniedText      = `text=Someone in the meeting has to let you in`

	// --- Speaker view ribbon (used by speakers_observer.go) ---------------
	SelActiveSpeakerTile = `[data-self-name][data-allocation-index]`

	// --- Misc -------------------------------------------------------------
	SelGotItButton = `button:has-text("Got it")`
	SelDismissDialogButton = `button[aria-label="Close"]`
)
