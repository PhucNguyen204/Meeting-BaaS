package meet

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/yourorg/meet-bot-go/internal/logic/browser"
)

// EnableMeetAudioCapture injects the in-page Web Audio API plumbing that
// captures every speaker's audio stream and pushes raw PCM frames to a
// JavaScript callback exposed via Playwright's exposeFunction binding.
//
// Port reference: src/meeting/meet/audio-capture.ts.
//
// The browser side does the heavy lifting (it has access to the Web Audio
// API and AudioContext.createMediaStreamSource); the Go side simply:
//   1. exposeFunction("__bot_pushAudio", goCallback) on the page.
//   2. evaluate(<jsScript>) to install the AudioContext + worklet + binding.
//
// goCallback receives frames the bot can forward to the recorder or
// streaming output.
//
// TODO(user): port the JS payload from audio-capture.ts. The body below
// outlines the wiring.
func EnableMeetAudioCapture(ctx context.Context, page browser.Page, onFrame func(pcmPtr []float32, sampleRate int)) error {
	if page == nil {
		return fmt.Errorf("meet: nil page")
	}
	log := zap.L().Named("meet.audio_capture")
	_ = log
	_ = onFrame
	_ = ctx

	// TODO(user):
	//   if err := page.ExposeFunction("__bot_pushAudio", func(_ ...any) any { ... }); err != nil { return err }
	//   _, err := page.Evaluate(audioCaptureJS)
	//   return err

	return fmt.Errorf("meet.EnableMeetAudioCapture: not implemented")
}

// audioCaptureJS is the in-page worklet/init script.
//
// TODO(user): paste the JS from src/meeting/meet/audio-capture.ts here.
const audioCaptureJS = `// TODO(user): port from src/meeting/meet/audio-capture.ts`
