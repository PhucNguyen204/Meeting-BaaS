package recorder

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"go.uber.org/zap"
)

// FinalizeOptions configures post-stop processing.
//
// Defaults are reasonable for a single-meeting bot session:
//
//	OutputDir = directory of RawPath
//	MP4Name   = "output.mp4"
//	WAVName   = "output.wav"
//	WAVSampleRate = 16000
//	ApplyOffset   = true (skip leading audio/video to align)
type FinalizeOptions struct {
	// RawPath is the on-disk file produced by ScreenRecorder (the "raw" mp4).
	RawPath string
	// OutputDir is where output.mp4 and output.wav are written. Defaults to RawPath dir.
	OutputDir string
	// MP4Name overrides the default "output.mp4".
	MP4Name string
	// WAVName overrides the default "output.wav".
	WAVName string
	// WAVSampleRate is the resample rate for the wav (default 16000 Hz).
	WAVSampleRate int
	// ApplyOffset, when true, calls CalculateOffset and seeks both files
	// so the audio beep and video flash align at t=0 in the output.
	ApplyOffset bool
}

// FinalizeResult captures the artefacts produced by Finalize.
type FinalizeResult struct {
	MP4Path    string
	WAVPath    string
	Offset     SyncOffset
	DurationS  float64
	OffsetUsed bool
}

// Finalize is the post-stop pipeline:
//
//  1. ffprobe the raw recording for duration metadata.
//  2. (optional) Compute audio/video sync offset via CalculateOffset.
//  3. Re-mux the raw mp4 to output.mp4 with -movflags +faststart so it can
//     be streamed before fully downloaded.
//  4. Extract output.wav: mono, 16 kHz, pcm_s16le (suitable for STT).
//
// Port reference: src/recording/screenRecorder.ts (the post-stop block) +
// src/utils/CalculVideoOffset.ts.
func Finalize(ctx context.Context, opts FinalizeOptions, log *zap.Logger) (*FinalizeResult, error) {
	if log == nil {
		log = zap.NewNop()
	}
	if opts.RawPath == "" {
		return nil, fmt.Errorf("recorder.Finalize: RawPath required")
	}
	if _, err := os.Stat(opts.RawPath); err != nil {
		return nil, fmt.Errorf("recorder.Finalize: stat %s: %w", opts.RawPath, err)
	}

	if opts.OutputDir == "" {
		opts.OutputDir = filepath.Dir(opts.RawPath)
	}
	if opts.MP4Name == "" {
		opts.MP4Name = "output.mp4"
	}
	if opts.WAVName == "" {
		opts.WAVName = "output.wav"
	}
	if opts.WAVSampleRate <= 0 {
		opts.WAVSampleRate = 16000
	}

	mp4Out := filepath.Join(opts.OutputDir, opts.MP4Name)
	wavOut := filepath.Join(opts.OutputDir, opts.WAVName)

	probe, err := Probe(ctx, opts.RawPath, log)
	if err != nil {
		log.Warn("ffprobe failed (continuing without duration)", zap.Error(err))
	}

	// Optional offset pass — the recorder injects a beep+flash at session start;
	// detect them and seek past so output.* aligns at t=0.
	var sync SyncOffset
	var offsetUsed bool
	if opts.ApplyOffset {
		// To compute offset we need a wav copy of the raw audio. Generate a
		// throwaway wav (same args as final wav) into the same dir.
		tmpWAV := filepath.Join(opts.OutputDir, "._sync_probe.wav")
		if err := extractWAV(ctx, opts.RawPath, tmpWAV, opts.WAVSampleRate, log); err == nil {
			defer os.Remove(tmpWAV)
			sync, _ = CalculateOffset(ctx, tmpWAV, opts.RawPath, log)
			if sync.Confidence >= 0.5 {
				offsetUsed = true
			}
		} else {
			log.Warn("sync probe wav failed, skipping offset", zap.Error(err))
		}
	}

	if err := remuxMP4(ctx, opts.RawPath, mp4Out, sync, offsetUsed, log); err != nil {
		return nil, fmt.Errorf("recorder.Finalize: mp4: %w", err)
	}
	if err := extractWAVWithOffset(ctx, opts.RawPath, wavOut, opts.WAVSampleRate, sync, offsetUsed, log); err != nil {
		return nil, fmt.Errorf("recorder.Finalize: wav: %w", err)
	}

	log.Info("finalize complete",
		zap.String("mp4", mp4Out),
		zap.String("wav", wavOut),
		zap.Float64("duration_s", probe.Duration),
		zap.Float64("offset_s", sync.OffsetSeconds),
		zap.Bool("offset_applied", offsetUsed),
	)
	return &FinalizeResult{
		MP4Path:    mp4Out,
		WAVPath:    wavOut,
		Offset:     sync,
		DurationS:  probe.Duration,
		OffsetUsed: offsetUsed,
	}, nil
}

// remuxMP4 copies the raw mp4 into a faststart-enabled output.mp4. When
// offsetUsed is true the audio is delayed by sync.OffsetSeconds via -itsoffset
// so the audio beep aligns with the video flash.
//
// The video stream is copy'd (no re-encode) for speed; the audio stream is
// re-encoded to AAC because mp4 cannot carry pcm_s16le.
func remuxMP4(ctx context.Context, in, out string, sync SyncOffset, offsetUsed bool, log *zap.Logger) error {
	args := []string{"-y"}
	if offsetUsed && sync.OffsetSeconds != 0 {
		// Negative offset → seek audio forward; positive → delay.
		// Using -itsoffset on the audio input track.
		args = append(args, "-itsoffset", strconv.FormatFloat(sync.OffsetSeconds, 'f', 6, 64))
	}
	args = append(args,
		"-i", in,
		"-c:v", "copy",
		"-c:a", "aac",
		"-b:a", "192k",
		"-movflags", "+faststart",
		out,
	)

	log.Debug("ffmpeg remux", zap.Strings("args", args))
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	out2, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg remux: %w (%s)", err, truncate(string(out2), 400))
	}
	return nil
}

// extractWAV is the simple form (no offset). Used during the sync probe pass.
func extractWAV(ctx context.Context, in, out string, sampleRate int, log *zap.Logger) error {
	args := []string{
		"-y",
		"-i", in,
		"-vn",
		"-ac", "1",
		"-ar", strconv.Itoa(sampleRate),
		"-c:a", "pcm_s16le",
		out,
	}
	log.Debug("ffmpeg wav extract", zap.Strings("args", args))
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	o, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg wav: %w (%s)", err, truncate(string(o), 400))
	}
	return nil
}

// extractWAVWithOffset extracts a mono pcm_s16le wav, optionally seeking past
// the calibration beep so t=0 aligns with the video flash.
func extractWAVWithOffset(ctx context.Context, in, out string, sampleRate int, sync SyncOffset, offsetUsed bool, log *zap.Logger) error {
	args := []string{"-y"}
	// If video flash arrives at videoTimestamp, we want to start the wav at
	// audioTimestamp so audio[0] corresponds to video[0] (= flash).
	if offsetUsed && sync.AudioTimestamp > 0 {
		args = append(args, "-ss", strconv.FormatFloat(sync.AudioTimestamp, 'f', 6, 64))
	}
	args = append(args,
		"-i", in,
		"-vn",
		"-ac", "1",
		"-ar", strconv.Itoa(sampleRate),
		"-c:a", "pcm_s16le",
		out,
	)
	log.Debug("ffmpeg wav extract", zap.Strings("args", args))
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	o, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg wav: %w (%s)", err, truncate(string(o), 400))
	}
	return nil
}

func truncate(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
