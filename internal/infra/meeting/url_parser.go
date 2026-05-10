package meeting

// url_parser.go contains the public ParseURL helper that dispatches to
// the per-provider parser. Each provider package supplies its own concrete
// implementation (see meet/url_parser.go) ÃƒÂ¢Ã¢â€šÂ¬Ã¢â‚¬Â this file only houses the
// dispatch logic so callers don't need to know which provider package to
// import in order to parse a URL.

import (
	"context"

	"github.com/PhucNguyen204/Meeting-BaaS/internal/config"
)

// ParseFunc is the signature provider packages use to register their parser.
type ParseFunc func(ctx context.Context, rawURL string) (MeetingInfo, error)

// parsers maps a MeetingProvider to its parser. Populated via Register
// from each provider package's init() block.
//
// Using a registry keeps the meeting/meeting package free from cyclic
// imports of every provider.
//
//nolint:gochecknoglobals // registry initialised at package init
var parsers = map[config.MeetingProvider]ParseFunc{}

// Register associates a parser with a provider. Last-write-wins; intended
// to be called exactly once per provider package init.
func Register(p config.MeetingProvider, fn ParseFunc) {
	parsers[p] = fn
}

// ParseURL detects the provider then runs its parser. Convenience wrapper.
//
// Returns ErrInvalidMeetingURL via DetectProvider if the URL is unrecognised.
func ParseURL(ctx context.Context, rawURL string) (config.MeetingProvider, MeetingInfo, error) {
	provider, err := DetectProvider(rawURL)
	if err != nil {
		return "", MeetingInfo{}, err
	}
	fn, ok := parsers[provider]
	if !ok {
		// Provider known but no parser registered yet (Phase 1 only ships Meet).
		return provider, MeetingInfo{}, nil
	}
	info, err := fn(ctx, rawURL)
	return provider, info, err
}
