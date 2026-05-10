package meet

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/playwright-community/playwright-go"
	"go.uber.org/zap"

	"github.com/PhucNguyen204/Meeting-BaaS/internal/domain"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/infra/speaker"
)

// jsSpeakerData mirrors the JS payload pushed by the in-page MutationObserver.
//
// Port reference: src/meeting/meet/speakersObserver.ts SpeakerData.
type jsSpeakerData struct {
	Name      string `json:"name"`
	ID        int64  `json:"id"`
	Speaking  bool   `json:"isSpeaking"`
	Timestamp int64  `json:"timestamp"`
}

// SpeakersObserver watches the Meet UI for active speakers and forwards
// SpeakerEvent records to the speaker manager (which the recorder uses
// to overlay names and adjust the layout).
//
// Port reference: src/meeting/meet/speakersObserver.ts +
// src/meeting/speakersObserver.ts.
type SpeakersObserver struct {
	mgr *speaker.Manager
	log *zap.Logger

	mu       sync.Mutex
	enabled  bool
	exposeOK bool
	startAt  time.Time
}

// NewSpeakersObserver constructs an observer that publishes events to mgr.
func NewSpeakersObserver(log *zap.Logger, mgr *speaker.Manager) *SpeakersObserver {
	if log == nil {
		log = zap.NewNop()
	}
	return &SpeakersObserver{mgr: mgr, log: log.Named("meet.speakers")}
}

// Attach installs the in-page observer on the given page. Safe to call
// multiple times; subsequent calls are no-ops.
//
// Port reference: src/meeting/meet/speakersObserver.ts MeetSpeakersObserver.startObserving.
func (o *SpeakersObserver) Attach(ctx context.Context, page domain.Page) error {
	if page == nil {
		return fmt.Errorf("speakers: nil page")
	}
	o.mu.Lock()
	if o.enabled {
		o.mu.Unlock()
		return nil
	}
	o.startAt = time.Now()
	o.mu.Unlock()

	o.log.Info("attaching meet speakers observer")

	if !o.exposeOK {
		if err := page.ExposeFunction(meetSpeakersCallbackName, o.handle); err != nil {
			o.log.Debug("expose function returned error", zap.Error(err))
		}
		o.exposeOK = true
	}

	scriptContent := meetSpeakersObserverJS
	if err := page.AddInitScript(playwright.Script{Content: &scriptContent}); err != nil {
		return fmt.Errorf("speakers: add init script: %w", err)
	}

	o.mu.Lock()
	o.enabled = true
	o.mu.Unlock()

	_ = ctx
	return nil
}

// handle bridges the in-page binding to the speaker manager. The JS payload
// posts an array of SpeakerData; we decode and Push each to the manager.
func (o *SpeakersObserver) handle(args ...any) any {
	if len(args) == 0 || o.mgr == nil {
		return nil
	}
	raw, err := json.Marshal(args[0])
	if err != nil {
		o.log.Debug("speakers payload re-encode failed", zap.Error(err))
		return nil
	}
	var speakers []jsSpeakerData
	if err := json.Unmarshal(raw, &speakers); err != nil {
		o.log.Debug("speakers payload decode failed", zap.Error(err))
		return nil
	}

	now := time.Now()
	o.mu.Lock()
	start := o.startAt
	o.mu.Unlock()

	for _, sp := range speakers {
		o.mgr.Push(context.Background(), speaker.SpeakerEvent{
			Name:              sp.Name,
			ParticipantID:     fmt.Sprintf("%d", sp.ID),
			Active:            sp.Speaking,
			At:                now,
			AtMillisFromStart: now.Sub(start).Milliseconds(),
		})
	}
	return nil
}

const meetSpeakersCallbackName = "meetSpeakersChanged"

// meetSpeakersObserverJS is a simplified port of the in-page MutationObserver
// payload from src/meeting/meet/speakersObserver.ts.
//
// Strategy:
//   - Subscribe to DOM mutations on the meeting layout root.
//   - Detect the active-speaker tile by its data-* attributes / class names.
//   - Debounce rapid mutations (50 ms) and emit a SpeakerData[] snapshot.
//
// Note: the full TS implementation (~700 lines) supports gallery_view, freeze
// detection and iframe traversal. This port covers the common case
// (speaker_view single-iframe). Extending to gallery is tracked in Phase 5.
const meetSpeakersObserverJS = `
(function() {
    if (window.__meetSpeakersObserverInstalled) return;
    window.__meetSpeakersObserverInstalled = true;

    const DEBOUNCE_MS = 50;
    let lastEmit = 0;
    let pendingTimer = null;
    let lastPayload = '';

    function collectSpeakers() {
        const speakers = [];
        // Active-speaker tiles: Meet marks them with data-self-name + data-allocation-index;
        // active speakers also have a class that includes "speaking" / animated border.
        const tiles = document.querySelectorAll('[data-self-name][data-allocation-index]');
        const seenNames = new Set();
        tiles.forEach(t => {
            const name = (t.getAttribute('data-self-name') || '').trim();
            if (!name || seenNames.has(name)) return;
            seenNames.add(name);
            // Heuristics for "is speaking":
            //   - aria-label contains "is speaking" / "talking"
            //   - has a child element with class containing "speaking" / "PpUOge"
            //   - has data-* attribute indicating active audio level
            let isSpeaking = false;
            const aria = (t.getAttribute('aria-label') || '').toLowerCase();
            if (aria.includes('is speaking') || aria.includes('talking') || aria.includes('audio')) isSpeaking = true;
            const html = t.innerHTML || '';
            if (!isSpeaking && /class="[^"]*\bspeak/i.test(html)) isSpeaking = true;
            const idStr = t.getAttribute('data-allocation-index') || '0';
            speakers.push({
                name: name,
                id: parseInt(idStr, 10) || 0,
                isSpeaking: isSpeaking,
                timestamp: Date.now(),
            });
        });
        return speakers;
    }

    function emit() {
        try {
            const speakers = collectSpeakers();
            const payload = JSON.stringify(speakers);
            if (payload === lastPayload) return;
            lastPayload = payload;
            if (typeof window.meetSpeakersChanged === 'function') {
                window.meetSpeakersChanged(speakers);
            }
        } catch (e) {
            console.error('[MeetSpeakers] emit error:', e);
        }
    }

    function scheduleEmit() {
        const now = Date.now();
        if (pendingTimer) return;
        const delay = Math.max(0, DEBOUNCE_MS - (now - lastEmit));
        pendingTimer = setTimeout(() => {
            pendingTimer = null;
            lastEmit = Date.now();
            emit();
        }, delay);
    }

    const observer = new MutationObserver((mutations) => {
        // Only react to attribute / childlist changes that may indicate UI update.
        for (const m of mutations) {
            if (m.type === 'attributes' || m.type === 'childList') {
                scheduleEmit();
                break;
            }
        }
    });

    function startWhenReady() {
        if (!document.body) {
            setTimeout(startWhenReady, 200);
            return;
        }
        observer.observe(document.body, {
            attributes: true,
            childList: true,
            subtree: true,
            attributeFilter: ['class', 'aria-label', 'data-allocation-index'],
        });
        // Initial emit after layout settles.
        setTimeout(emit, 1500);
        // Periodic re-check (covers cases where mutations are missed).
        setInterval(emit, 5000);
        console.log('[MeetSpeakers] observer started');
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', startWhenReady);
    } else {
        startWhenReady();
    }
})();
`
