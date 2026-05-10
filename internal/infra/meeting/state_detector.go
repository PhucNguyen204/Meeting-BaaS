package meeting

import (
	"context"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/domain"
	"time"
)

// StateConfig declares how to detect a high-level meeting state from
// per-state CSS selectors.
//
// Port reference: src/utils/meeting-state-detector.ts +
// src/meeting/meet-state-config.ts +
// src/meeting/teams-state-config.ts.
//
// Each provider supplies a StateConfig at startup; the StateDetector then
// evaluates selectors against the live page and returns the first state
// whose selector list resolves to a visible element.
type StateConfig struct {
	// Provider name (purely informational, used in logs).
	Provider string

	// States is an ordered list ÃƒÂ¢Ã¢â€šÂ¬Ã¢â‚¬Â the detector returns the first match.
	States []StateRule
}

// StateRule maps a logical state to one or more CSS selectors.
//
// Multiple selectors are evaluated as logical OR.
type StateRule struct {
	// Name of the state ("waiting_room", "in_call", "removed", "lobby_open", ...).
	Name string

	// AnyOf is a list of selectors. The state matches if any locator
	// resolves to a visible element.
	AnyOf []string

	// EvaluateTimeout caps how long the locator is allowed to settle.
	// Zero defaults to 1s.
	EvaluateTimeout time.Duration
}

// Detector evaluates a StateConfig against a live Page.
//
// Construct one detector per provider (it is essentially stateless; only
// the StateConfig differs between providers).
type Detector struct {
	cfg StateConfig
}

// NewDetector returns a Detector bound to cfg.
func NewDetector(cfg StateConfig) *Detector {
	return &Detector{cfg: cfg}
}

// Detect returns the first matching state, or empty string if none match.
//
// Port reference: src/utils/meeting-state-detector.ts.
func (d *Detector) Detect(ctx context.Context, page domain.Page) (string, error) {
	if d == nil || page == nil {
		return "", nil
	}

	for _, rule := range d.cfg.States {
		timeout := rule.EvaluateTimeout
		if timeout <= 0 {
			timeout = time.Second
		}

		for _, sel := range rule.AnyOf {
			loc := page.Locator(sel)

			// Use a short timeout context per selector evaluation to avoid
			// blocking the state machine loop.
			evalCtx, cancel := context.WithTimeout(ctx, timeout)
			visible, err := loc.IsVisible()
			cancel()

			if err != nil {
				// Selector evaluation errors are non-fatal; skip to next.
				continue
			}
			if visible {
				return rule.Name, nil
			}

			// Check if parent context was cancelled.
			if evalCtx.Err() != nil {
				return "", ctx.Err()
			}
		}
	}
	return "", nil
}
