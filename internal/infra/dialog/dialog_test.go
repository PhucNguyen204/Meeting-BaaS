package dialog

import (
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	obs := New(nil)
	if obs == nil {
		t.Fatal("expected non-nil observer")
	}
}

func TestObserver_Subscribe_Nil(t *testing.T) {
	obs := New(nil)
	// Should not panic.
	obs.Subscribe(nil)
	if len(obs.handlers) != 0 {
		t.Error("nil handler should not be added")
	}
}

func TestObserver_Subscribe_Valid(t *testing.T) {
	obs := New(nil)
	obs.Subscribe(func(ev Event) bool { return false })
	if len(obs.handlers) != 1 {
		t.Errorf("expected 1 handler, got %d", len(obs.handlers))
	}
}

func TestObserver_Dispatch(t *testing.T) {
	obs := New(nil)
	var received Event
	obs.Subscribe(func(ev Event) bool {
		received = ev
		return true
	})

	ev := Event{
		Title:    "Test Dialog",
		Body:     "This is a test",
		Selector: "#test",
		Severity: SeverityWarn,
		At:       time.Now(),
	}
	autoDismiss := obs.dispatch(ev)
	if !autoDismiss {
		t.Error("expected autoDismiss=true")
	}
	if received.Title != "Test Dialog" {
		t.Errorf("got title %q", received.Title)
	}
}

func TestObserver_Dispatch_NoDismiss(t *testing.T) {
	obs := New(nil)
	obs.Subscribe(func(_ Event) bool { return false })
	obs.Subscribe(func(_ Event) bool { return false })

	autoDismiss := obs.dispatch(Event{Title: "test"})
	if autoDismiss {
		t.Error("expected autoDismiss=false when no handler returns true")
	}
}

func TestObserver_Dispatch_Mixed(t *testing.T) {
	obs := New(nil)
	obs.Subscribe(func(_ Event) bool { return false })
	obs.Subscribe(func(_ Event) bool { return true })
	obs.Subscribe(func(_ Event) bool { return false })

	autoDismiss := obs.dispatch(Event{Title: "test"})
	if !autoDismiss {
		t.Error("expected autoDismiss=true when any handler returns true")
	}
}

func TestObserver_Dispatch_NoHandlers(t *testing.T) {
	obs := New(nil)
	autoDismiss := obs.dispatch(Event{Title: "test"})
	if autoDismiss {
		t.Error("expected autoDismiss=false with no handlers")
	}
}

func TestObserver_Dispatch_HandlerOrder(t *testing.T) {
	obs := New(nil)
	var order []int
	obs.Subscribe(func(_ Event) bool { order = append(order, 1); return false })
	obs.Subscribe(func(_ Event) bool { order = append(order, 2); return false })
	obs.Subscribe(func(_ Event) bool { order = append(order, 3); return false })

	obs.dispatch(Event{})
	if len(order) != 3 || order[0] != 1 || order[1] != 2 || order[2] != 3 {
		t.Errorf("handlers called in wrong order: %v", order)
	}
}

func TestObserver_Attach_NilPage(t *testing.T) {
	obs := New(nil)
	err := obs.Attach(nil, nil)
	if err != nil {
		t.Errorf("nil page should return nil: %v", err)
	}
}

func TestObserver_Attach_NilObserver(t *testing.T) {
	var obs *Observer
	err := obs.Attach(nil, nil)
	if err != nil {
		t.Errorf("nil observer should return nil: %v", err)
	}
}

func TestSeverity_Values(t *testing.T) {
	if SeverityInfo != 0 || SeverityWarn != 1 || SeverityFatal != 2 {
		t.Error("severity iota order unexpected")
	}
}
