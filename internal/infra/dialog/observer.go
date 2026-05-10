package dialog

import (
	"context"
	"github.com/PhucNguyen204/Meeting-BaaS/internal/domain"
	"sync"

	"go.uber.org/zap"
)

// Observer watches a Page for popup dialogs and dispatches matching events
// to subscribed handlers.
//
// Port reference: src/services/dialog-observer/simple-dialog-observer.ts.
//
// Implementation strategy (TS):
//   - Inject a MutationObserver via page.AddInitScript that watches for
//     elements matching a list of dialog selectors.
//   - When a dialog appears, the in-page script calls the exposed binding
//     __bot_dialogEvent(<json>) which lands here.
//   - This Go side dispatches to handlers in registration order; a handler
//     may signal autoDismiss to have the observer click the close button.
//
// Construct one Observer per page. Safe for concurrent Subscribe / Unsubscribe.
type Observer struct {
	log *zap.Logger

	mu       sync.RWMutex
	handlers []Handler
}

// New returns a fresh observer. Call Attach(page) to start receiving events.
func New(log *zap.Logger) *Observer {
	if log == nil {
		log = zap.NewNop()
	}
	return &Observer{log: log.Named("dialog")}
}

// Subscribe adds a handler. Handlers are invoked in the order they were
// registered.
func (o *Observer) Subscribe(h Handler) {
	if h == nil {
		return
	}
	o.mu.Lock()
	o.handlers = append(o.handlers, h)
	o.mu.Unlock()
}

// Attach wires the in-page MutationObserver to page. May be called once
// per page; subsequent calls are no-ops.
//
// TODO(user): port the JS payload + binding from
// src/services/dialog-observer/simple-dialog-observer.ts.
func (o *Observer) Attach(ctx context.Context, page domain.Page) error {
	if page == nil || o == nil {
		return nil
	}
	o.log.Info("attaching dialog observer")

	// TODO(user):
	//   _ = page.ExposeFunction("__bot_dialogEvent", o.bridge)
	//   _, err := page.AddInitScript(playwright.Script{Content: dialogObserverJS})
	//   return err

	_ = ctx
	return nil
}

// bridge is invoked by the in-page binding. The first arg is the JSON
// event payload as a string.
//
// TODO(user): JSON-decode args[0] into Event and call dispatch.
func (o *Observer) bridge(args ...any) any {
	o.log.Debug("dialog event", zap.Any("payload", args))
	return nil
}

// dispatch calls every handler. If any handler returns autoDismiss=true
// the page-side script is told to click the close button.
func (o *Observer) dispatch(ev Event) (autoDismiss bool) {
	o.mu.RLock()
	hs := append([]Handler(nil), o.handlers...)
	o.mu.RUnlock()
	for _, h := range hs {
		if h(ev) {
			autoDismiss = true
		}
	}
	return autoDismiss
}

// dialogObserverJS is the in-page MutationObserver payload.
//
// TODO(user): paste from src/services/dialog-observer/simple-dialog-observer.ts.
const dialogObserverJS = `// TODO(user): port from src/services/dialog-observer/simple-dialog-observer.ts`
