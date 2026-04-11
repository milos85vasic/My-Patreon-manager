package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/assert"
)

func TestRateLimiterEvictsStaleEntries(t *testing.T) {
	clock := clockwork.NewFakeClock()
	rl := NewIPRateLimiter(2, 4, 1*time.Minute).WithClock(clock)
	for i := 0; i < 100; i++ {
		rl.Allow(fmt.Sprintf("ip-%d", i))
	}
	if rl.Len() != 100 {
		t.Fatalf("expected 100 entries, got %d", rl.Len())
	}
	clock.Advance(90 * time.Second)
	rl.Sweep()
	if rl.Len() != 0 {
		t.Fatalf("expected 0 after sweep, got %d", rl.Len())
	}
}

func TestRateLimiterUpdatesSeenOnAllow(t *testing.T) {
	clock := clockwork.NewFakeClock()
	rl := NewIPRateLimiter(10, 10, 1*time.Minute).WithClock(clock)
	rl.Allow("a")
	clock.Advance(30 * time.Second)
	rl.Allow("a") // refresh
	clock.Advance(40 * time.Second)
	rl.Sweep()
	if rl.Len() != 1 {
		t.Fatalf("expected 1 (still fresh), got %d", rl.Len())
	}
}

func TestRateLimiterUpdatesSeenOnGetLimiter(t *testing.T) {
	clock := clockwork.NewFakeClock()
	rl := NewIPRateLimiter(10, 10, 1*time.Minute).WithClock(clock)
	_ = rl.GetLimiter("a")
	clock.Advance(30 * time.Second)
	_ = rl.GetLimiter("a") // refresh via GetLimiter path
	clock.Advance(40 * time.Second)
	rl.Sweep()
	if rl.Len() != 1 {
		t.Fatalf("expected 1 (still fresh via GetLimiter), got %d", rl.Len())
	}
}

func TestRateLimiterSweepLeavesFresh(t *testing.T) {
	clock := clockwork.NewFakeClock()
	rl := NewIPRateLimiter(10, 10, 1*time.Minute).WithClock(clock)
	rl.Allow("old")
	clock.Advance(90 * time.Second)
	rl.Allow("new")
	rl.Sweep()
	if rl.Len() != 1 {
		t.Fatalf("expected 1 (new survives, old evicted), got %d", rl.Len())
	}
}

func TestRateLimiterDefaultTTL(t *testing.T) {
	// No TTL arg => default TTL applied.
	rl := NewIPRateLimiter(1, 1)
	if rl.ttl != defaultIPTTL {
		t.Fatalf("expected default TTL %v, got %v", defaultIPTTL, rl.ttl)
	}
	// Zero TTL arg => default TTL applied.
	rl2 := NewIPRateLimiter(1, 1, 0)
	if rl2.ttl != defaultIPTTL {
		t.Fatalf("expected default TTL when 0 passed, got %v", rl2.ttl)
	}
}

func TestRateLimiterCleanupStaleExplicitMaxAge(t *testing.T) {
	clock := clockwork.NewFakeClock()
	rl := NewIPRateLimiter(10, 10, 1*time.Hour).WithClock(clock)
	rl.Allow("a")
	clock.Advance(2 * time.Second)
	// TTL is 1h so Sweep would NOT evict; explicit maxAge=1s should evict.
	rl.CleanupStale(1 * time.Second)
	if rl.Len() != 0 {
		t.Fatalf("expected 0 after CleanupStale with small maxAge, got %d", rl.Len())
	}
}

func TestIPRateLimiter_LimitMethod(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rl := NewIPRateLimiter(1, 1)
	engine := gin.New()
	engine.Use(rl.Limit())
	engine.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })

	// First request OK.
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/ping", nil)
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Second from same IP => 429.
	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/ping", nil)
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Equal(t, "60", w.Header().Get("Retry-After"))
	assert.Contains(t, w.Body.String(), "rate limit exceeded")
}

func TestIPRateLimiter_ConcurrentSweep(t *testing.T) {
	clock := clockwork.NewFakeClock()
	rl := NewIPRateLimiter(100, 100, 1*time.Minute).WithClock(clock)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				rl.Allow(fmt.Sprintf("worker-%d-%d", id, j))
			}
		}(i)
	}
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rl.Sweep()
		}()
	}
	wg.Wait()
	// No assertion on count (races by design); just ensure no panic/deadlock
	// and that Len is callable.
	_ = rl.Len()
}
