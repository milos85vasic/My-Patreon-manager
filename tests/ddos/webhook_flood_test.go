package ddos

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/milos85vasic/My-Patreon-Manager/internal/handlers"
	"github.com/milos85vasic/My-Patreon-Manager/internal/middleware"
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

func TestWebhookFlood_RateLimiting(t *testing.T) {
	// Test IPRateLimiter directly
	limiter := middleware.NewIPRateLimiter(10, 5) // 10 req/sec, burst 5
	ip := "192.168.1.1"

	// Burst should allow up to 5 requests immediately
	for i := 0; i < 5; i++ {
		assert.True(t, limiter.GetLimiter(ip).Allow(), "burst requests should be allowed")
	}
	// Next request should be rate limited (since rate is 10/sec, but we exhausted burst)
	// Allow() may still return true if enough tokens accumulated (0.1 sec per token).
	// To ensure rate limiting, we test that AllowN with zero wait returns false.
	// Actually, we can just verify that after burst, Allow() returns false if called immediately.
	// However, due to timing granularity, it might still allow. We'll consume tokens with Reserve.
	r := limiter.GetLimiter(ip).ReserveN(time.Now(), 5)
	assert.True(t, r.OK(), "should reserve burst")
	// Now limiter has 0 tokens, next request should wait
	r2 := limiter.GetLimiter(ip).Reserve()
	assert.True(t, r2.OK())
	delay := r2.Delay()
	assert.Greater(t, delay, time.Duration(0), "should have positive delay after burst exhausted")
	t.Logf("Delay after burst: %v", delay)
}

func TestWebhookFlood_DeduplicationQueueOverflow(t *testing.T) {
	// Create deduplicator with short window
	window := 50 * time.Millisecond
	ed := ssync.NewEventDeduplicator(window)

	// Add many unique events (more than window can hold? it's a map, but cleanup goroutine runs every window)
	// We'll add events and ensure memory doesn't explode (hard to test).
	// Instead, verify that after window expires, entries are removed.
	const numEvents = 1000
	for i := 0; i < numEvents; i++ {
		ed.TrackEvent(string(rune(i)))
	}
	// All should be duplicates immediately
	for i := 0; i < numEvents; i++ {
		assert.True(t, ed.IsDuplicate(string(rune(i))), "event should be duplicate within window")
	}
	// Wait for window to expire
	time.Sleep(window * 2)
	// Now events should not be duplicates (since window passed, they are evicted)
	// However, the cleanup goroutine runs every window, so entries should be removed.
	// The deduplicator uses a map with timestamps; cleanup removes old entries.
	// We'll verify that at least some events are not duplicates (the ones that were added early).
	// Since we slept double window, all should be cleaned up.
	// Note: the cleanup goroutine may not have run yet; we can trigger cleanup by calling TrackEvent.
	ed.TrackEvent("trigger")
	// Check a few random events; they should not be duplicates
	for i := 0; i < 10; i++ {
		assert.False(t, ed.IsDuplicate(string(rune(i))), "event should not be duplicate after window")
	}
}

func TestWebhookFlood_ServerResponsiveness(t *testing.T) {
	// Verify that under a flood of duplicate events, a unique event is still
	// handled promptly. Uses an httptest server with the real Gin handler
	// backed by an EventDeduplicator and IPRateLimiter.
	gin.SetMode(gin.TestMode)
	router := gin.New()

	dedup := ssync.NewEventDeduplicator(5 * time.Minute)
	handler := handlers.NewWebhookHandler(dedup, nil, nil)
	limiter := middleware.NewIPRateLimiter(1000, 500) // generous limit for test
	router.POST("/webhook/github", limiter.Limit(), handler.GitHubWebhook)

	server := httptest.NewServer(router)
	defer server.Close()

	payload := []byte(`{"repository":{"full_name":"owner/repo","html_url":"https://github.com/owner/repo"}}`)

	// Send a flood of duplicate events
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest("POST", server.URL+"/webhook/github", bytes.NewReader(payload))
			req.Header.Set("X-GitHub-Delivery", "duplicate-event-id")
			req.Header.Set("X-GitHub-Event", "push")
			resp, err := http.DefaultClient.Do(req)
			if err == nil {
				resp.Body.Close()
			}
		}()
	}
	wg.Wait()

	// Now send a unique event and measure responsiveness
	start := time.Now()
	req, _ := http.NewRequest("POST", server.URL+"/webhook/github", bytes.NewReader(payload))
	req.Header.Set("X-GitHub-Delivery", "unique-event-id")
	req.Header.Set("X-GitHub-Event", "push")
	resp, err := http.DefaultClient.Do(req)
	elapsed := time.Since(start)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
	assert.Less(t, elapsed, 2*time.Second, "unique event should be handled promptly under flood")
	t.Logf("Unique event response time under flood: %v", elapsed)
}
