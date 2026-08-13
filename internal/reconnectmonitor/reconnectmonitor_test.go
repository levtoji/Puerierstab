package reconnectmonitor

import (
	"testing"
	"time"
)

func TestNoWarnUnderThreshold(t *testing.T) {
	m := New(3, 10*time.Minute)
	for i := 0; i < 2; i++ {
		m.OnResumed(nil)
	}
	if len(m.events) != 2 {
		t.Fatalf("expected 2 recorded events, got %d", len(m.events))
	}
}

func TestPrunesOldEvents(t *testing.T) {
	m := New(3, 10*time.Minute)
	old := time.Now().Add(-11 * time.Minute)
	m.mu.Lock()
	m.events = append(m.events, old)
	m.mu.Unlock()

	m.OnResumed(nil)
	m.mu.Lock()
	count := len(m.events)
	m.mu.Unlock()
	if count != 1 {
		t.Fatalf("expected old event pruned, got %d events", count)
	}
}

func TestKeepsAllInWindow(t *testing.T) {
	m := New(3, 10*time.Minute)
	for i := 0; i < 5; i++ {
		m.OnResumed(nil)
	}
	m.mu.Lock()
	count := len(m.events)
	m.mu.Unlock()
	if count != 5 {
		t.Fatalf("expected all 5 in-window events recorded, got %d", count)
	}
}
