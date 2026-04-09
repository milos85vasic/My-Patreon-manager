package sync_test

import (
	"testing"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/services/sync"
	"github.com/stretchr/testify/assert"
)

func TestEventDeduplicator_NewEvent(t *testing.T) {
	ed := sync.NewEventDeduplicator(5 * time.Minute)
	assert.False(t, ed.IsDuplicate("event-1"))
}

func TestEventDeduplicator_TrackAndDuplicate(t *testing.T) {
	ed := sync.NewEventDeduplicator(5 * time.Minute)
	ed.TrackEvent("event-1")
	assert.True(t, ed.IsDuplicate("event-1"))
	assert.False(t, ed.IsDuplicate("event-2"))
}

func TestEventDeduplicator_WindowExpiry(t *testing.T) {
	ed := sync.NewEventDeduplicator(10 * time.Millisecond)
	ed.TrackEvent("event-1")
	assert.True(t, ed.IsDuplicate("event-1"))

	time.Sleep(20 * time.Millisecond)
	assert.False(t, ed.IsDuplicate("event-1"))
}

func TestEventDeduplicator_MultipleEvents(t *testing.T) {
	ed := sync.NewEventDeduplicator(5 * time.Minute)
	ed.TrackEvent("e1")
	ed.TrackEvent("e2")
	ed.TrackEvent("e3")

	assert.True(t, ed.IsDuplicate("e1"))
	assert.True(t, ed.IsDuplicate("e2"))
	assert.True(t, ed.IsDuplicate("e3"))
	assert.False(t, ed.IsDuplicate("e4"))
}

func TestEventDeduplicator_ReTrack(t *testing.T) {
	ed := sync.NewEventDeduplicator(5 * time.Minute)
	ed.TrackEvent("e1")
	ed.TrackEvent("e1")
	assert.True(t, ed.IsDuplicate("e1"))
}
