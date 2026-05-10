package recorder

import (
	"fmt"
	"strconv"
)

// FFmpegBuilder constructs FFmpeg command arguments using the Builder
// design pattern. Ensures consistent, testable argument composition.
type FFmpegBuilder struct {
	args []string
}

// NewFFmpegBuilder creates a new builder with common defaults.
func NewFFmpegBuilder() *FFmpegBuilder {
	return &FFmpegBuilder{
		args: []string{"-y"}, // overwrite output
	}
}

// Input adds a video input source.
func (b *FFmpegBuilder) Input(format, source string) *FFmpegBuilder {
	b.args = append(b.args, "-f", format, "-i", source)
	return b
}

// AudioInput adds an audio input source.
func (b *FFmpegBuilder) AudioInput(format, source string) *FFmpegBuilder {
	b.args = append(b.args, "-f", format, "-i", source)
	return b
}

// VideoCodec sets the video codec.
func (b *FFmpegBuilder) VideoCodec(codec string) *FFmpegBuilder {
	b.args = append(b.args, "-c:v", codec)
	return b
}

// AudioCodec sets the audio codec.
func (b *FFmpegBuilder) AudioCodec(codec string) *FFmpegBuilder {
	b.args = append(b.args, "-c:a", codec)
	return b
}

// Preset sets the encoding preset (ultrafast, fast, medium, etc).
func (b *FFmpegBuilder) Preset(preset string) *FFmpegBuilder {
	b.args = append(b.args, "-preset", preset)
	return b
}

// CRF sets the constant rate factor (0-51, lower = better quality).
func (b *FFmpegBuilder) CRF(crf int) *FFmpegBuilder {
	b.args = append(b.args, "-crf", strconv.Itoa(crf))
	return b
}

// AudioBitrate sets the audio bitrate.
func (b *FFmpegBuilder) AudioBitrate(bitrate string) *FFmpegBuilder {
	b.args = append(b.args, "-b:a", bitrate)
	return b
}

// Resolution sets the output resolution.
func (b *FFmpegBuilder) Resolution(width, height int) *FFmpegBuilder {
	b.args = append(b.args, "-s", fmt.Sprintf("%dx%d", width, height))
	return b
}

// FPS sets the frame rate.
func (b *FFmpegBuilder) FPS(fps int) *FFmpegBuilder {
	b.args = append(b.args, "-r", strconv.Itoa(fps))
	return b
}

// PixelFormat sets the pixel format.
func (b *FFmpegBuilder) PixelFormat(pf string) *FFmpegBuilder {
	b.args = append(b.args, "-pix_fmt", pf)
	return b
}

// OutputPath sets the output file path.
func (b *FFmpegBuilder) OutputPath(path string) *FFmpegBuilder {
	b.args = append(b.args, path)
	return b
}

// Raw adds arbitrary arguments.
func (b *FFmpegBuilder) Raw(args ...string) *FFmpegBuilder {
	b.args = append(b.args, args...)
	return b
}

// Build returns the final argument list.
func (b *FFmpegBuilder) Build() []string {
	return append([]string(nil), b.args...)
}
