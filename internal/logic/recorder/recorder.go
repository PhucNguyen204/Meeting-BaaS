// Package recorder manages FFmpeg screen and audio recording processes.
//
// Port reference: src/recording/screenRecorder.ts.
package recorder

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ScreenRecorder manages an FFmpeg subprocess that captures the virtual
// display (Xvfb) and PulseAudio output into an MP4 file.
//
// Port reference: src/recording/screenRecorder.ts.
type ScreenRecorder struct {
	mu         sync.Mutex
	log        *zap.Logger
	cmd        *exec.Cmd
	outputPath string
	display    string
	startedAt  time.Time
	running    bool
}

// Options configures the recorder.
type Options struct {
	OutputPath string // e.g. /tmp/recording.mp4
	Display    string // e.g. ":99" — defaults to DISPLAY env
	Width      int
	Height     int
	FPS        int
	AudioInput string // PulseAudio source, e.g. "default"
}

// New constructs a ScreenRecorder.
func New(log *zap.Logger, opts Options) *ScreenRecorder {
	if log == nil {
		log = zap.NewNop()
	}
	if opts.Display == "" {
		opts.Display = ":99"
	}
	if opts.Width == 0 {
		opts.Width = 1280
	}
	if opts.Height == 0 {
		opts.Height = 720
	}
	if opts.FPS == 0 {
		opts.FPS = 30
	}
	if opts.AudioInput == "" {
		opts.AudioInput = "default"
	}
	if opts.OutputPath == "" {
		opts.OutputPath = "/tmp/recording.mp4"
	}

	return &ScreenRecorder{
		log:        log.Named("recorder"),
		outputPath: opts.OutputPath,
		display:    opts.Display,
	}
}

// Start launches the FFmpeg subprocess.
func (r *ScreenRecorder) Start(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.running {
		return fmt.Errorf("recorder: already running")
	}

	args := NewFFmpegBuilder().
		Input("x11grab", r.display).
		AudioInput("pulse", "default").
		VideoCodec("libx264").
		Preset("ultrafast").
		CRF(23).
		AudioCodec("aac").
		AudioBitrate("128k").
		OutputPath(r.outputPath).
		Build()

	r.log.Info("starting ffmpeg", zap.Strings("args", args))

	r.cmd = exec.CommandContext(ctx, "ffmpeg", args...)
	if err := r.cmd.Start(); err != nil {
		return fmt.Errorf("recorder: start ffmpeg: %w", err)
	}

	r.running = true
	r.startedAt = time.Now()
	r.log.Info("ffmpeg started", zap.String("output", r.outputPath))
	return nil
}

// Stop sends SIGINT to FFmpeg for graceful finalization.
func (r *ScreenRecorder) Stop(_ context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.running || r.cmd == nil || r.cmd.Process == nil {
		return nil
	}

	r.log.Info("stopping ffmpeg", zap.Duration("duration", time.Since(r.startedAt)))

	// Send 'q' to ffmpeg stdin for graceful stop.
	if r.cmd.Process != nil {
		if err := r.cmd.Process.Signal(signalInterrupt()); err != nil {
			r.log.Warn("signal ffmpeg failed, killing", zap.Error(err))
			_ = r.cmd.Process.Kill()
		}
	}

	if err := r.cmd.Wait(); err != nil {
		r.log.Warn("ffmpeg exit", zap.Error(err))
	}

	r.running = false
	r.log.Info("ffmpeg stopped")
	return nil
}

// Pause sends SIGSTOP to suspend FFmpeg.
func (r *ScreenRecorder) Pause(_ context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.running || r.cmd == nil || r.cmd.Process == nil {
		return nil
	}
	return r.cmd.Process.Signal(signalStop())
}

// Resume sends SIGCONT to resume FFmpeg.
func (r *ScreenRecorder) Resume(_ context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.running || r.cmd == nil || r.cmd.Process == nil {
		return nil
	}
	return r.cmd.Process.Signal(signalContinue())
}

// OutputPath returns the recording file path.
func (r *ScreenRecorder) OutputPath() string { return r.outputPath }
