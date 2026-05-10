package recorder

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
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

// SyncOffset is the audio/video sync offset returned by CalculateOffset.
//
// Port reference: src/utils/CalculVideoOffset.ts SyncOffset.
type SyncOffset struct {
	// AudioTimestamp is the time of the detected audio beep, in seconds.
	AudioTimestamp float64
	// VideoTimestamp is the time of the detected video flash, in seconds.
	VideoTimestamp float64
	// OffsetSeconds is video - audio (positive ⇒ video is ahead).
	OffsetSeconds float64
	// Confidence in [0, 1]; 0.9 when both signals detected, 0.1 fallback.
	Confidence float64
}

const analysisWindowSec = 10

// CalculateOffset computes the audio/video sync offset using ffmpeg
// silence-detect (audio) and scene-detect (video). On any failure returns
// a low-confidence zero offset.
//
// Port reference: src/utils/CalculVideoOffset.ts calculateVideoOffset().
func CalculateOffset(ctx context.Context, audioPath, videoPath string, log *zap.Logger) (SyncOffset, error) {
	if log == nil {
		log = zap.NewNop()
	}
	log.Info("calculating av offset",
		zap.String("audio", audioPath),
		zap.String("video", videoPath),
		zap.Int("window_sec", analysisWindowSec),
	)

	audioTs := detectAudioBeep(ctx, audioPath, log)
	videoTs := detectVideoFlash(ctx, videoPath, log)

	if audioTs <= 0 || videoTs <= 0 {
		log.Warn("sync signal missing, using default offset",
			zap.Float64("audio_ts", audioTs),
			zap.Float64("video_ts", videoTs),
		)
		return SyncOffset{
			AudioTimestamp: maxFloat(audioTs, 0),
			VideoTimestamp: maxFloat(videoTs, 0),
			OffsetSeconds:  0,
			Confidence:     0.1,
		}, nil
	}

	off := SyncOffset{
		AudioTimestamp: audioTs,
		VideoTimestamp: videoTs,
		OffsetSeconds:  videoTs - audioTs,
		Confidence:     0.9,
	}
	log.Info("av sync offset",
		zap.Float64("audio_ts", audioTs),
		zap.Float64("video_ts", videoTs),
		zap.Float64("offset_s", off.OffsetSeconds),
		zap.Float64("confidence", off.Confidence),
	)
	return off, nil
}

// detectAudioBeep runs ffmpeg silencedetect over the first analysisWindowSec
// of the audio file and returns the first silence_end timestamp (the beep
// onset). Returns 0 if no signal is found.
func detectAudioBeep(ctx context.Context, audioPath string, log *zap.Logger) float64 {
	args := []string{
		"-i", audioPath,
		"-af", "silencedetect=noise=-35dB:duration=0.01",
		"-f", "null",
		"-t", strconv.Itoa(analysisWindowSec),
		"-",
	}
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	out, _ := cmd.CombinedOutput() // ffmpeg writes silencedetect events to stderr
	re := regexp.MustCompile(`silence_end:\s*([0-9.]+)`)
	matches := re.FindAllStringSubmatch(string(out), -1)
	for _, m := range matches {
		t, err := strconv.ParseFloat(m[1], 64)
		if err == nil && t > 0.01 && t < float64(analysisWindowSec) {
			return t
		}
	}
	if log != nil {
		log.Debug("no audio beep detected", zap.Int("matches", len(matches)))
	}
	return 0
}

// detectVideoFlash runs ffmpeg scene detection on the first analysisWindowSec
// of the video and returns the timestamp of the largest scene change > 0.5s.
// Returns 0 if no signal is found.
func detectVideoFlash(ctx context.Context, videoPath string, log *zap.Logger) float64 {
	args := []string{
		"-i", videoPath,
		"-vf", "select='gt(scene,0.05)',showinfo",
		"-vsync", "0",
		"-f", "null",
		"-t", strconv.Itoa(analysisWindowSec),
		"-",
	}
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	out, _ := cmd.CombinedOutput()
	timeRe := regexp.MustCompile(`pts_time:([0-9.]+)`)
	sceneRe := regexp.MustCompile(`scene:([0-9.]+)`)

	var best float64
	var bestScene float64
	timeMatches := timeRe.FindAllStringSubmatch(string(out), -1)
	sceneMatches := sceneRe.FindAllStringSubmatch(string(out), -1)
	for i, m := range timeMatches {
		t, err := strconv.ParseFloat(m[1], 64)
		if err != nil || t <= 0.5 || t >= float64(analysisWindowSec) {
			continue
		}
		var sceneVal float64
		if i < len(sceneMatches) {
			sceneVal, _ = strconv.ParseFloat(sceneMatches[i][1], 64)
		}
		if sceneVal > bestScene {
			best = t
			bestScene = sceneVal
		}
	}
	if log != nil {
		log.Debug("video scene scan", zap.Float64("best_t", best), zap.Float64("best_scene", bestScene))
	}
	return best
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
