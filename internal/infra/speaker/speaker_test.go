package speaker

import (
	"context"
	"testing"
	"time"
)

func TestNewManager(t *testing.T) {
	m := NewManager(nil)
	if m == nil {
		t.Fatal("expected non-nil manager")
	}
}

func TestManager_Push_Active(t *testing.T) {
	m := NewManager(nil)
	m.Push(context.Background(), SpeakerEvent{
		Name:   "Alice",
		Active: true,
		At:     time.Now(),
	})

	snap := m.Snapshot()
	if len(snap.ActiveSpeakers) != 1 || snap.ActiveSpeakers[0] != "Alice" {
		t.Errorf("expected Alice active, got %v", snap.ActiveSpeakers)
	}
	if len(snap.AllParticipants) != 1 || snap.AllParticipants[0] != "Alice" {
		t.Errorf("expected Alice in all, got %v", snap.AllParticipants)
	}
}

func TestManager_Push_Inactive(t *testing.T) {
	m := NewManager(nil)
	m.Push(context.Background(), SpeakerEvent{Name: "Bob", Active: true})
	m.Push(context.Background(), SpeakerEvent{Name: "Bob", Active: false})

	snap := m.Snapshot()
	if len(snap.ActiveSpeakers) != 0 {
		t.Errorf("expected no active speakers, got %v", snap.ActiveSpeakers)
	}
	if len(snap.AllParticipants) != 1 {
		t.Errorf("expected Bob in all participants, got %v", snap.AllParticipants)
	}
}

func TestManager_Push_EmptyName(t *testing.T) {
	m := NewManager(nil)
	m.Push(context.Background(), SpeakerEvent{Name: "", Active: true})
	snap := m.Snapshot()
	if len(snap.AllParticipants) != 0 {
		t.Error("empty name should be ignored")
	}
}

func TestManager_Push_NilManager(t *testing.T) {
	var m *Manager
	// Should not panic.
	m.Push(context.Background(), SpeakerEvent{Name: "X", Active: true})
}

func TestManager_Snapshot_NilManager(t *testing.T) {
	var m *Manager
	snap := m.Snapshot()
	if len(snap.ActiveSpeakers) != 0 || len(snap.AllParticipants) != 0 {
		t.Error("nil manager snapshot should be empty")
	}
}

func TestManager_Subscribe(t *testing.T) {
	m := NewManager(nil)
	events, cancel := m.Subscribe(10)
	defer cancel()

	m.Push(context.Background(), SpeakerEvent{Name: "Charlie", Active: true})

	select {
	case ev := <-events:
		if ev.Name != "Charlie" {
			t.Errorf("got name %q, want Charlie", ev.Name)
		}
		if !ev.Active {
			t.Error("expected active=true")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestManager_Subscribe_Cancel(t *testing.T) {
	m := NewManager(nil)
	events, cancel := m.Subscribe(10)
	cancel()

	// Channel should be closed after cancel.
	_, ok := <-events
	if ok {
		t.Error("expected channel to be closed after cancel")
	}

	// Pushing after cancel should not panic.
	m.Push(context.Background(), SpeakerEvent{Name: "Dave", Active: true})
}

func TestManager_MultipleSubscribers(t *testing.T) {
	m := NewManager(nil)
	events1, cancel1 := m.Subscribe(10)
	events2, cancel2 := m.Subscribe(10)
	defer cancel1()
	defer cancel2()

	m.Push(context.Background(), SpeakerEvent{Name: "Eve", Active: true})

	check := func(ch <-chan SpeakerEvent, label string) {
		select {
		case ev := <-ch:
			if ev.Name != "Eve" {
				t.Errorf("%s: got name %q", label, ev.Name)
			}
		case <-time.After(time.Second):
			t.Fatalf("%s: timeout", label)
		}
	}
	check(events1, "sub1")
	check(events2, "sub2")
}

func TestManager_Subscribe_DefaultBufSize(t *testing.T) {
	m := NewManager(nil)
	events, cancel := m.Subscribe(0) // 0 â†’ default 64
	defer cancel()

	m.Push(context.Background(), SpeakerEvent{Name: "Frank", Active: true})

	select {
	case ev := <-events:
		if ev.Name != "Frank" {
			t.Errorf("got %q", ev.Name)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestManager_MultipleActiveSpeakers(t *testing.T) {
	m := NewManager(nil)
	m.Push(context.Background(), SpeakerEvent{Name: "A", Active: true})
	m.Push(context.Background(), SpeakerEvent{Name: "B", Active: true})
	m.Push(context.Background(), SpeakerEvent{Name: "C", Active: true})

	snap := m.Snapshot()
	if len(snap.ActiveSpeakers) != 3 {
		t.Errorf("expected 3 active, got %d", len(snap.ActiveSpeakers))
	}
	if len(snap.AllParticipants) != 3 {
		t.Errorf("expected 3 total, got %d", len(snap.AllParticipants))
	}
}
