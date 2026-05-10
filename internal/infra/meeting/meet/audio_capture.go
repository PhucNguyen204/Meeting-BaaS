package meet

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/playwright-community/playwright-go"
	"go.uber.org/zap"

	"github.com/PhucNguyen204/Meeting-BaaS/internal/domain"
)

// AudioChunk is the in-page audio frame the Web Audio mixer pushes to Go.
//
// Port reference: src/meeting/shared/audio-capture.ts (callback payload).
type AudioChunk struct {
	// AudioData is the mono mixed PCM samples as Float32 in range [-1, 1].
	// Length == NumberOfFrames; one channel only (browser mixes stereo down).
	AudioData []float32 `json:"audioData"`
	// SampleRate is the browser's reported AudioContext rate (typically 48000).
	SampleRate int `json:"sampleRate"`
	// Timestamp is the AudioFrame timestamp in microseconds (browser clock).
	Timestamp float64 `json:"timestamp"`
	// NumberOfFrames is the length of AudioData.
	NumberOfFrames int `json:"numberOfFrames"`
}

// AudioCallback receives every mixed audio chunk from the in-page worklet.
//
// Implementations should be cheap (fan-out, drop-on-full) — the Playwright
// dispatcher loop blocks while the callback runs.
type AudioCallback func(AudioChunk)

// AudioCapture wires the Web Audio API mixer in the meeting page to a Go
// callback. The mixer:
//
//  1. Intercepts every RTCPeerConnection in the page.
//  2. Pulls the audio track from each peer connection.
//  3. Routes them through an AudioContext destination (browser mixes them).
//  4. Pushes Float32 PCM frames to window.onMeetMixedAudioChunk.
//  5. The exposed Go function decodes and forwards to AudioCallback.
//
// Port reference: src/meeting/meet/audio-capture.ts +
// src/meeting/shared/audio-capture.ts (provider="Meet").
type AudioCapture struct {
	log      *zap.Logger
	callback AudioCallback

	mu       sync.Mutex
	enabled  bool
	exposeOK bool // page.ExposeFunction succeeded once (it must not run twice)
}

// NewAudioCapture constructs a capture wired to the given callback.
//
// Pass nil callback to discard chunks (useful for smoke tests).
func NewAudioCapture(log *zap.Logger, cb AudioCallback) *AudioCapture {
	if log == nil {
		log = zap.NewNop()
	}
	return &AudioCapture{log: log.Named("meet.audio"), callback: cb}
}

// Enable installs the in-page mixer + Go binding on page. Must be called
// once per page; subsequent calls are no-ops.
//
// Port reference: src/meeting/shared/audio-capture.ts createAudioCapture(...).enable.
func (c *AudioCapture) Enable(ctx context.Context, page domain.Page) error {
	if page == nil {
		return fmt.Errorf("meet.audio: nil page")
	}
	c.mu.Lock()
	if c.enabled {
		c.mu.Unlock()
		c.log.Debug("audio capture already enabled, skipping")
		return nil
	}
	c.mu.Unlock()

	// 1. Expose the Go callback so the JS payload can call window.onMeetMixedAudioChunk(chunk).
	if !c.exposeOK {
		err := page.ExposeFunction(meetAudioCallbackName, c.handleChunk)
		if err != nil {
			// Playwright errors with "has been already registered" if the
			// page is reused. Treat as success (parity with TS).
			c.log.Debug("expose function returned error", zap.Error(err))
		}
		c.exposeOK = true
	}

	// 2. Inject the mixer script. AddInitScript persists across SPA navigations.
	scriptContent := meetAudioCaptureJS
	if err := page.AddInitScript(playwright.Script{Content: &scriptContent}); err != nil {
		return fmt.Errorf("meet.audio: add init script: %w", err)
	}

	c.mu.Lock()
	c.enabled = true
	c.mu.Unlock()

	c.log.Info("meet audio capture enabled")
	_ = ctx
	return nil
}

// Stop calls the in-page graceful shutdown function. Safe to call multiple times.
func (c *AudioCapture) Stop(_ context.Context, page domain.Page) error {
	if page == nil {
		return nil
	}
	expr := fmt.Sprintf(`async () => { if (typeof window.%s === 'function') return window.%s(); }`,
		meetAudioStopFunctionName, meetAudioStopFunctionName)
	if _, err := page.Evaluate(expr); err != nil {
		c.log.Warn("stop audio capture failed", zap.Error(err))
		return err
	}
	c.log.Info("meet audio capture stopped")
	return nil
}

// handleChunk is the Playwright-side bridge from JS to Go. It receives the
// JSON-decoded chunk as args[0] (a map[string]any), re-encodes via
// encoding/json into AudioChunk, and dispatches to the user callback.
func (c *AudioCapture) handleChunk(args ...any) any {
	if len(args) == 0 {
		return nil
	}
	raw, err := json.Marshal(args[0])
	if err != nil {
		c.log.Debug("audio chunk re-encode failed", zap.Error(err))
		return nil
	}
	var chunk AudioChunk
	if err := json.Unmarshal(raw, &chunk); err != nil {
		c.log.Debug("audio chunk decode failed", zap.Error(err))
		return nil
	}
	if c.callback != nil {
		c.callback(chunk)
	}
	return nil
}

// Bindings names match the JS payload below. Keep in sync if the script changes.
const (
	meetAudioCallbackName     = "onMeetMixedAudioChunk"
	meetAudioStopFunctionName = "__meetAudioStop"
)

// meetAudioCaptureJS is the in-page Web Audio mixer.
//
// Port reference: src/meeting/shared/audio-capture.ts generateAudioCaptureScript
// (provider="Meet", enablePeriodicScanning=false).
//
// The script:
//   - Creates an AudioContext + MediaStreamDestination.
//   - Wraps RTCPeerConnection so every audio track is routed through the mixer.
//   - Reads pre-mixed frames via MediaStreamTrackProcessor and pushes them to
//     window.onMeetMixedAudioChunk.
//
// Stop is exposed as window.__meetAudioStop().
const meetAudioCaptureJS = `
(function() {
    try {
        console.log('[MeetAudio] Initializing Web Audio mixer...');
        const audioCtx = new (window.AudioContext || window.webkitAudioContext)();
        const mixerDestination = audioCtx.createMediaStreamDestination();
        const mixedAudioSources = new Map();
        let mixedStreamProcessor = null;
        let chunksSent = 0;
        let abortController = null;
        let processorPromise = null;

        async function startMixedStreamProcessor() {
            if (mixedStreamProcessor) return;
            const mixedTrack = mixerDestination.stream.getAudioTracks()[0];
            if (!mixedTrack) {
                console.error('[MeetAudio] No mixed audio track available');
                return;
            }
            try {
                if (typeof MediaStreamTrackProcessor === 'undefined') {
                    console.error('[MeetAudio] MediaStreamTrackProcessor not available');
                    return;
                }
                const processor = new MediaStreamTrackProcessor({ track: mixedTrack });
                const reader = processor.readable.getReader();
                mixedStreamProcessor = reader;
                abortController = new AbortController();
                const signal = abortController.signal;
                console.log('[MeetAudio] Started Web Audio mixed stream processor');

                const processFrames = async (signal) => {
                    let currentFrame = null;
                    const onAbort = () => { reader.cancel().catch(() => {}); };
                    signal.addEventListener('abort', onAbort);
                    try {
                        while (true) {
                            if (signal.aborted) break;
                            const { done, value: frame } = await reader.read();
                            if (done) break;
                            if (signal.aborted) { if (frame) frame.close(); break; }
                            if (!frame) continue;
                            currentFrame = frame;
                            try {
                                const numChannels = frame.numberOfChannels;
                                const numSamples = frame.numberOfFrames;
                                const audioData = new Float32Array(numSamples);
                                if (numChannels > 1) {
                                    const channelData = new Float32Array(numSamples);
                                    for (let channel = 0; channel < numChannels; channel++) {
                                        frame.copyTo(channelData, { planeIndex: channel });
                                        for (let i = 0; i < numSamples; i++) audioData[i] += channelData[i];
                                    }
                                    for (let i = 0; i < numSamples; i++) audioData[i] /= numChannels;
                                } else {
                                    frame.copyTo(audioData, { planeIndex: 0 });
                                }
                                if (typeof window.onMeetMixedAudioChunk === 'function') {
                                    window.onMeetMixedAudioChunk({
                                        audioData: Array.from(audioData),
                                        sampleRate: frame.sampleRate,
                                        timestamp: frame.timestamp,
                                        numberOfFrames: numSamples,
                                    });
                                    chunksSent++;
                                    if (chunksSent === 1) console.log('[MeetAudio] First audio chunk sent to Go');
                                    else if (chunksSent % 100 === 0) console.log('[MeetAudio] Sent ' + chunksSent + ' chunks');
                                }
                                frame.close();
                                currentFrame = null;
                            } catch (err) {
                                console.error('[MeetAudio] Frame processing error:', err);
                                if (currentFrame) { try { currentFrame.close(); } catch (e) {} currentFrame = null; }
                            }
                        }
                    } finally {
                        signal.removeEventListener('abort', onAbort);
                        if (currentFrame) { try { currentFrame.close(); } catch (e) {} }
                        try { reader.releaseLock(); } catch (e) {}
                        mixedStreamProcessor = null;
                        console.log('[MeetAudio] Processor cleanup complete, sent ' + chunksSent + ' total chunks');
                    }
                };
                processorPromise = processFrames(signal);
            } catch (e) {
                console.error('[MeetAudio] Failed to start mixed stream processor:', e);
            }
        }

        async function stopMixedStreamProcessor() {
            if (abortController) {
                console.log('[MeetAudio] Stopping mixed stream processor...');
                abortController.abort();
                if (processorPromise) await processorPromise;
                abortController = null;
                processorPromise = null;
                console.log('[MeetAudio] Mixed stream processor stopped');
            }
        }
        window.__meetAudioStop = stopMixedStreamProcessor;

        window.addEventListener('beforeunload', () => { stopMixedStreamProcessor(); });
        document.addEventListener('visibilitychange', () => {
            if (document.visibilityState === 'hidden') stopMixedStreamProcessor();
        });

        function connectTrackToMixer(track) {
            if (mixedAudioSources.has(track.id)) return;
            try {
                if (audioCtx.state === 'suspended') audioCtx.resume();
                const stream = new MediaStream([track]);
                const source = audioCtx.createMediaStreamSource(stream);
                source.connect(mixerDestination);
                mixedAudioSources.set(track.id, source);
                console.log('[MeetAudio] Connected track ' + track.id + ' (' + mixedAudioSources.size + ' total)');
                if (mixedAudioSources.size === 1) startMixedStreamProcessor();
                track.onended = () => {
                    source.disconnect();
                    mixedAudioSources.delete(track.id);
                };
            } catch (e) {
                console.error('[MeetAudio] Failed to connect track to mixer:', e);
            }
        }

        if (typeof window.RTCPeerConnection !== 'undefined') {
            const OriginalPC = window.RTCPeerConnection;
            window.RTCPeerConnection = function (...args) {
                const pc = new OriginalPC(...args);
                pc.addEventListener('track', (event) => {
                    if (event.track.kind === 'audio') {
                        console.log('[MeetAudio] Audio track detected:', event.track.id);
                        connectTrackToMixer(event.track);
                    }
                });
                return pc;
            };
            console.log('[MeetAudio] RTCPeerConnection intercepted');
        }
        console.log('[MeetAudio] Web Audio mixer initialized');
    } catch (e) {
        console.error('[MeetAudio] Fatal Error:', e);
    }
})();
`

// EnableMeetAudioCapture is the legacy free-function entrypoint kept for
// backward compatibility with callers that don't want to manage an
// AudioCapture instance. New code should use NewAudioCapture / Enable.
//
// Port reference: src/meeting/meet/audio-capture.ts enableMeetAudioCapture.
func EnableMeetAudioCapture(ctx context.Context, page domain.Page, onFrame AudioCallback) error {
	cap := NewAudioCapture(zap.L().Named("meet.audio_capture"), onFrame)
	return cap.Enable(ctx, page)
}
