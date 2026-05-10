// Package dialog provides a small abstraction over the in-page dialog
// observer used by Meet/Teams to detect popup banners (cookie consent,
// "video unavailable", "join from another device", etc) and either
// dismiss them or surface them as state machine events.
//
// Port reference: src/services/dialog-observer/types.ts +
// src/services/dialog-observer/simple-dialog-observer.ts.
package dialog

import "time"

// Severity helps the state machine decide what to do with a dialog event.
type Severity int

const (
	SeverityInfo  Severity = iota // banners and tooltips
	SeverityWarn                  // recoverable popups (cookie consent, video off)
	SeverityFatal                 // bot-must-leave popups (meeting ended, removed)
)

// Event is what the in-page observer reports.
type Event struct {
	// Title is the heading the user sees, when present.
	Title string
	// Body is the dialog body text.
	Body string
	// Selector that matched, useful for debugging.
	Selector string
	// Severity classifies the dialog.
	Severity Severity
	// At is when the event was raised (in browser-side wallclock time).
	At time.Time
}

// Handler reacts to an Event. Returning true means the observer should
// proceed to dismiss the dialog automatically (click the "Got it" /
// "Continue" / "Close" button). Returning false leaves the DOM untouched.
type Handler func(Event) (autoDismiss bool)
