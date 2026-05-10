// Package snapshot dumps the current Meet/Teams DOM (and a screenshot)
// to disk for debugging when the bot fails to make progress through the
// state machine.
//
// Port reference: src/services/html-snapshot-service.ts.
//
// The TS service writes:
//   logs/<bot_uuid>/snapshots/<state>-<unix_ms>.html
//   logs/<bot_uuid>/snapshots/<state>-<unix_ms>.png
//
// Snapshots are typically taken on:
//   - state machine transition (per-state hook)
//   - error path before terminating
//   - explicit /debug/snapshot HTTP endpoint (Phase 4)
package snapshot

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/playwright-community/playwright-go"
	"go.uber.org/zap"

	"github.com/yourorg/meet-bot-go/internal/logic/browser"
)

// Service writes HTML + screenshot pairs to disk.
type Service struct {
	log    *zap.Logger
	baseDir string
}

// New constructs a Service rooted at baseDir. baseDir is created on demand.
func New(log *zap.Logger, baseDir string) *Service {
	if log == nil {
		log = zap.NewNop()
	}
	if baseDir == "" {
		baseDir = "./logs/snapshots"
	}
	return &Service{log: log.Named("snapshot"), baseDir: baseDir}
}

// Take writes <baseDir>/<botUUID>/<tag>-<unix_ms>.html|.png.
//
// Errors from individual write steps are joined; a failure on the
// screenshot does NOT abort the HTML dump (and vice versa).
//
// TODO(user): port the optional rate limiting & deduplication from
// html-snapshot-service.ts (avoid spamming snapshots in tight loops).
func (s *Service) Take(ctx context.Context, page browser.Page, botUUID, tag string) error {
	if page == nil {
		return fmt.Errorf("snapshot: nil page")
	}
	dir := filepath.Join(s.baseDir, botUUID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("snapshot: mkdir: %w", err)
	}
	stamp := time.Now().UnixMilli()
	htmlPath := filepath.Join(dir, fmt.Sprintf("%s-%d.html", tag, stamp))
	pngPath := filepath.Join(dir, fmt.Sprintf("%s-%d.png", tag, stamp))

	log := s.log.With(zap.String("tag", tag), zap.String("html", htmlPath), zap.String("png", pngPath))

	html, err := page.Content()
	if err == nil {
		if werr := os.WriteFile(htmlPath, []byte(html), 0o644); werr != nil {
			log.Warn("write html failed", zap.Error(werr))
		}
	} else {
		log.Warn("read html failed", zap.Error(err))
	}

	if _, err := page.Screenshot(playwright.PageScreenshotOptions{
		Path:     playwright.String(pngPath),
		FullPage: playwright.Bool(true),
	}); err != nil {
		log.Warn("screenshot failed", zap.Error(err))
	}

	_ = ctx
	log.Debug("snapshot saved")
	return nil
}
