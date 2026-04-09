package sync

import (
	"sync"
	"time"
)

type EventDeduplicator struct {
	mu     sync.RWMutex
	seen   map[string]time.Time
	window time.Duration
}

func NewEventDeduplicator(window time.Duration) *EventDeduplicator {
	ed := &EventDeduplicator{
		seen:   make(map[string]time.Time),
		window: window,
	}
	go ed.cleanup()
	return ed
}

func (ed *EventDeduplicator) TrackEvent(eventID string) {
	ed.mu.Lock()
	defer ed.mu.Unlock()
	ed.seen[eventID] = time.Now()
}

func (ed *EventDeduplicator) IsDuplicate(eventID string) bool {
	ed.mu.RLock()
	defer ed.mu.RUnlock()
	t, exists := ed.seen[eventID]
	if !exists {
		return false
	}
	return time.Since(t) < ed.window
}

func (ed *EventDeduplicator) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		ed.mu.Lock()
		now := time.Now()
		for id, t := range ed.seen {
			if now.Sub(t) > ed.window {
				delete(ed.seen, id)
			}
		}
		ed.mu.Unlock()
	}
}
