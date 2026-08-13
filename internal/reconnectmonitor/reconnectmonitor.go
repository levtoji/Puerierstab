package reconnectmonitor

import (
	"log/slog"
	"sync"
	"time"

	"github.com/disgoorg/disgo/events"
)

type Monitor struct {
	mu        sync.Mutex
	events    []time.Time
	threshold int
	window    time.Duration
}

func New(threshold int, window time.Duration) *Monitor {
	return &Monitor{
		events:    make([]time.Time, 0, threshold),
		threshold: threshold,
		window:    window,
	}
}

func (m *Monitor) OnResumed(event *events.Resumed) {
	now := time.Now()
	m.mu.Lock()
	m.events = append(m.events, now)
	cutoff := now.Add(-m.window)
	kept := m.events[:0]
	for _, ts := range m.events {
		if ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}
	m.events = kept
	count := len(m.events)
	exceeded := count >= m.threshold
	m.mu.Unlock()

	if exceeded {
		slog.Warn("gateway reconnect storm detected",
			slog.Int("reconnects", count),
			slog.Duration("window", m.window),
			slog.Int("threshold", m.threshold),
		)
	}
}
