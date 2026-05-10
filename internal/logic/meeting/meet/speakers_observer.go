package meet

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/yourorg/meet-bot-go/internal/logic/browser"
	"github.com/yourorg/meet-bot-go/internal/logic/speaker"
)

// SpeakersObserver watches the Meet UI for active speakers and forwards
// SpeakerEvent records to the speaker manager (which the recorder uses
// to overlay names and adjust the layout).
//
// Port reference: src/meeting/meet/speakersObserver.ts +
// src/meeting/speakersObserver.ts.
//
// Wiring:
//
//   1. ExposeFunction("__bot_speakerEvent", goCallback)
//   2. AddInitScript(<MutationObserver JS>)
//
// The JS payload watches the [data-self-name][data-allocation-index]
// attribute changes (selector [SelActiveSpeakerTile]) and emits events
// when participants begin/stop talking.
type SpeakersObserver struct {
	mgr *speaker.Manager
	log *zap.Logger
}

// NewSpeakersObserver constructs an observer that publishes events to mgr.
func NewSpeakersObserver(log *zap.Logger, mgr *speaker.Manager) *SpeakersObserver {
	if log == nil {
		log = zap.NewNop()
	}
	return &SpeakersObserver{mgr: mgr, log: log.Named("meet.speakers")}
}

// Attach installs the in-page observer on the given page.
//
// TODO(user): port the JS payload + binding from speakersObserver.ts.
func (o *SpeakersObserver) Attach(ctx context.Context, page browser.Page) error {
	if page == nil {
		return fmt.Errorf("speakers: nil page")
	}
	o.log.Info("attaching meet speakers observer")

	// TODO(user):
	//   if err := page.ExposeFunction("__bot_speakerEvent", o.handle); err != nil { return err }
	//   _, err := page.AddInitScript(playwright.Script{Content: speakersObserverJS})
	//   return err

	_ = ctx
	return nil
}

// handle bridges the in-page binding to the speaker manager.
//
// TODO(user): when implementing, decode the JSON payload and call
// o.mgr.Push(speaker.SpeakerEvent{...}).
func (o *SpeakersObserver) handle(args ...any) any {
	o.log.Debug("speaker event", zap.Any("payload", args))
	return nil
}

// speakersObserverJS is the in-page MutationObserver payload.
//
// TODO(user): paste from src/meeting/meet/speakersObserver.ts.
const speakersObserverJS = `// TODO(user): port from src/meeting/meet/speakersObserver.ts`
