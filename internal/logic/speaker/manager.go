package speaker

import (
	"context"
	"sync"

	"go.uber.org/zap"
)

// Manager aggregates SpeakerEvents from the in-page observer and exposes:
//
//   - Push(SpeakerEvent) — invoked by the observer.
//   - Subscribe() — recorder/streaming consumers receive a channel of events.
//   - Snapshot() — point-in-time view of active and known speakers.
//
// This Phase 1 skeleton implements only Push (logging) and Snapshot
// (returns zero-value). The recorder integration in Phase 2 will pull on
// subscribers and the timeline export in Phase 5 will marshal everything.
//
// Port reference: src/state-machine/states/recording-state.ts (consumer)
// + src/services/* (no direct TS analogue; the TS impl uses GLOBAL state).
type Manager struct {
	log *zap.Logger

	mu          sync.Mutex
	subscribers []chan SpeakerEvent

	// activeNames is the latest snapshot of names currently flagged as Active.
	// TODO(user): debounce/age-out logic per recording-state.ts:611-672.
	activeNames map[string]struct{}
	// allNames tracks every name ever seen.
	allNames map[string]struct{}
}

// NewManager constructs an empty Manager.
func NewManager(log *zap.Logger) *Manager {
	if log == nil {
		log = zap.NewNop()
	}
	return &Manager{
		log:         log.Named("speaker"),
		activeNames: make(map[string]struct{}),
		allNames:    make(map[string]struct{}),
	}
}

// Push records an event and fans it out to subscribers (non-blocking).
func (m *Manager) Push(_ context.Context, ev SpeakerEvent) {
	if m == nil || ev.Name == "" {
		return
	}
	m.mu.Lock()
	m.allNames[ev.Name] = struct{}{}
	if ev.Active {
		m.activeNames[ev.Name] = struct{}{}
	} else {
		delete(m.activeNames, ev.Name)
	}
	subs := append([]chan SpeakerEvent(nil), m.subscribers...)
	m.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- ev:
		default:
			m.log.Warn("speaker subscriber dropped event", zap.String("name", ev.Name))
		}
	}
}

// Subscribe returns a buffered channel of events. Unsubscribe by closing
// the returned cancel func; the channel will be closed afterwards.
//
// TODO(user): wire into the recording state in Phase 2.
func (m *Manager) Subscribe(buf int) (events <-chan SpeakerEvent, cancel func()) {
	if buf <= 0 {
		buf = 64
	}
	ch := make(chan SpeakerEvent, buf)

	m.mu.Lock()
	m.subscribers = append(m.subscribers, ch)
	m.mu.Unlock()

	return ch, func() {
		m.mu.Lock()
		for i, sub := range m.subscribers {
			if sub == ch {
				m.subscribers = append(m.subscribers[:i], m.subscribers[i+1:]...)
				break
			}
		}
		m.mu.Unlock()
		close(ch)
	}
}

// Snapshot returns a point-in-time view of speakers. Cheap to call.
func (m *Manager) Snapshot() Snapshot {
	if m == nil {
		return Snapshot{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	active := make([]string, 0, len(m.activeNames))
	for n := range m.activeNames {
		active = append(active, n)
	}
	all := make([]string, 0, len(m.allNames))
	for n := range m.allNames {
		all = append(all, n)
	}
	return Snapshot{
		ActiveSpeakers:  active,
		AllParticipants: all,
	}
}
