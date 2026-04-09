package ddos

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ssync "github.com/milos85vasic/My-Patreon-Manager/internal/services/sync"
	"github.com/stretchr/testify/assert"
)

func TestWebhookFlood_Deduplication(t *testing.T) {
	ed := ssync.NewEventDeduplicator(5 * time.Minute)

	totalSent := int64(1000)
	var deduplicated int64
	var unique int64

	var wg sync.WaitGroup
	for i := 0; i < int(totalSent); i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			eventID := "event-" + string(rune(id%100))
			if ed.IsDuplicate(eventID) {
				atomic.AddInt64(&deduplicated, 1)
			} else {
				atomic.AddInt64(&unique, 1)
				ed.TrackEvent(eventID)
			}
		}(i)
	}
	wg.Wait()

	t.Logf("Total: %d, Unique: %d, Deduplicated: %d", totalSent, unique, deduplicated)
}

func TestWebhookFlood_DedupWindow(t *testing.T) {
	ed := ssync.NewEventDeduplicator(50 * time.Millisecond)

	ed.TrackEvent("evt-1")
	assert.True(t, ed.IsDuplicate("evt-1"), "should be duplicate within window")

	time.Sleep(60 * time.Millisecond)
	assert.False(t, ed.IsDuplicate("evt-1"), "should not be duplicate after window")
}
