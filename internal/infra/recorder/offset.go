package recorder

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"go.uber.org/zap"
)

// ProbeResult holds the output of an ffprobe video analysis.
type ProbeResult struct {
	Duration float64 // seconds
	Width    int
	Height   int
}

// Probe runs ffprobe on the given file and returns metadata.
//
// Port reference: src/recording/screenRecorder.ts getVideoOffset() +
// src/recording/screenRecorder.ts getVideoDuration().
func Probe(ctx context.Context, path string, log *zap.Logger) (ProbeResult, error) {
	args := []string{
		"-v", "error",
		"-show_entries", "format=duration:stream=width,height",
		"-of", "csv=p=0",
		path,
	}

	cmd := exec.CommandContext(ctx, "ffprobe", args...)
	out, err := cmd.Output()
	if err != nil {
		return ProbeResult{}, fmt.Errorf("ffprobe: %w", err)
	}

	// Output format: "width,height\nduration"
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	result := ProbeResult{}

	for _, line := range lines {
		parts := strings.Split(strings.TrimSpace(line), ",")
		if len(parts) >= 2 {
			if w, err := strconv.Atoi(parts[0]); err == nil {
				result.Width = w
			}
			if h, err := strconv.Atoi(parts[1]); err == nil {
				result.Height = h
			}
		}
		if len(parts) == 1 {
			if d, err := strconv.ParseFloat(parts[0], 64); err == nil {
				result.Duration = d
			}
		}
	}

	if log != nil {
		log.Debug("ffprobe result",
			zap.String("path", path),
			zap.Float64("duration", result.Duration),
			zap.Int("width", result.Width),
			zap.Int("height", result.Height),
		)
	}
	return result, nil
}
