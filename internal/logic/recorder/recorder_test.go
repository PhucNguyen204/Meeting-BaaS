package recorder

import (
	"testing"
)

func TestNewFFmpegBuilder_Defaults(t *testing.T) {
	args := NewFFmpegBuilder().Build()
	if len(args) != 1 || args[0] != "-y" {
		t.Errorf("expected default [-y], got %v", args)
	}
}

func TestFFmpegBuilder_FullPipeline(t *testing.T) {
	args := NewFFmpegBuilder().
		Input("x11grab", ":99").
		AudioInput("pulse", "default").
		VideoCodec("libx264").
		Preset("ultrafast").
		CRF(23).
		AudioCodec("aac").
		AudioBitrate("128k").
		Resolution(1280, 720).
		FPS(30).
		PixelFormat("yuv420p").
		OutputPath("/tmp/out.mp4").
		Build()

	expected := []string{
		"-y",
		"-f", "x11grab", "-i", ":99",
		"-f", "pulse", "-i", "default",
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-crf", "23",
		"-c:a", "aac",
		"-b:a", "128k",
		"-s", "1280x720",
		"-r", "30",
		"-pix_fmt", "yuv420p",
		"/tmp/out.mp4",
	}

	if len(args) != len(expected) {
		t.Fatalf("expected %d args, got %d: %v", len(expected), len(args), args)
	}
	for i, want := range expected {
		if args[i] != want {
			t.Errorf("args[%d] = %q, want %q", i, args[i], want)
		}
	}
}

func TestFFmpegBuilder_Raw(t *testing.T) {
	args := NewFFmpegBuilder().
		Raw("-threads", "4").
		Build()
	if len(args) != 3 {
		t.Fatalf("expected 3 args, got %d", len(args))
	}
	if args[1] != "-threads" || args[2] != "4" {
		t.Errorf("raw args not appended correctly: %v", args)
	}
}

func TestFFmpegBuilder_BuildImmutable(t *testing.T) {
	b := NewFFmpegBuilder().VideoCodec("libx264")
	args1 := b.Build()
	args2 := b.Build()
	// Modifying args1 should not affect args2.
	args1[0] = "modified"
	if args2[0] == "modified" {
		t.Error("Build() should return independent copies")
	}
}

func TestFFmpegBuilder_Chainable(t *testing.T) {
	b := NewFFmpegBuilder()
	// Verify chaining returns the same builder.
	b2 := b.Input("a", "b").AudioInput("c", "d").VideoCodec("e").
		AudioCodec("f").Preset("g").CRF(0).AudioBitrate("h").
		Resolution(0, 0).FPS(0).PixelFormat("i").OutputPath("j").
		Raw("k")
	if b != b2 {
		t.Error("chaining should return the same builder instance")
	}
}

func TestNew_Defaults(t *testing.T) {
	rec := New(nil, Options{})
	if rec.outputPath != "/tmp/recording.mp4" {
		t.Errorf("default output path: %q", rec.outputPath)
	}
	if rec.display != ":99" {
		t.Errorf("default display: %q", rec.display)
	}
}

func TestNew_CustomOptions(t *testing.T) {
	rec := New(nil, Options{
		OutputPath: "/custom/path.mp4",
		Display:    ":42",
	})
	if rec.outputPath != "/custom/path.mp4" {
		t.Errorf("output path: %q", rec.outputPath)
	}
	if rec.display != ":42" {
		t.Errorf("display: %q", rec.display)
	}
}

func TestScreenRecorder_OutputPath(t *testing.T) {
	rec := New(nil, Options{OutputPath: "/test/out.mp4"})
	if rec.OutputPath() != "/test/out.mp4" {
		t.Errorf("OutputPath() = %q", rec.OutputPath())
	}
}

func TestScreenRecorder_StopNotRunning(t *testing.T) {
	rec := New(nil, Options{})
	// Stop when not running should be a no-op.
	if err := rec.Stop(nil); err != nil {
		t.Errorf("Stop on non-running recorder should be nil: %v", err)
	}
}

func TestScreenRecorder_PauseNotRunning(t *testing.T) {
	rec := New(nil, Options{})
	if err := rec.Pause(nil); err != nil {
		t.Errorf("Pause on non-running recorder should be nil: %v", err)
	}
}

func TestScreenRecorder_ResumeNotRunning(t *testing.T) {
	rec := New(nil, Options{})
	if err := rec.Resume(nil); err != nil {
		t.Errorf("Resume on non-running recorder should be nil: %v", err)
	}
}
