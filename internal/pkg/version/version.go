// Package version exposes build metadata injected by the linker.
//
// The Makefile injects values via -ldflags:
//
//	-X github.com/PhucNguyen204/Meeting-BaaS/internal/pkg/version.Version=...
//	-X github.com/PhucNguyen204/Meeting-BaaS/internal/pkg/version.Commit=...
//	-X github.com/PhucNguyen204/Meeting-BaaS/internal/pkg/version.BuildDate=...
//
// Port reference: src/buildInfo.json (TS counterpart). The HTTP /version
// handler returns this struct as JSON.
package version

// These vars are written by the linker. Do not modify at runtime.
//
//nolint:gochecknoglobals // build metadata is conventionally global
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

// Info bundles the build metadata for JSON serialization.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
}

// Get returns a snapshot of the build metadata.
func Get() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		BuildDate: BuildDate,
	}
}
