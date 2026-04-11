package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jonboulle/clockwork"
	"golang.org/x/time/rate"
)

// defaultIPTTL is the fallback TTL applied when NewIPRateLimiter is called
// without an explicit TTL (the legacy two-argument form). Entries untouched
// for this duration are eligible for eviction by Sweep.
const defaultIPTTL = 10 * time.Minute

// limiterEntry tracks the per-IP rate limiter plus the last-seen timestamp
// used for TTL eviction.
type limiterEntry struct {
	limiter *rate.Limiter
	seen    time.Time
}

// IPRateLimiter is a bounded-memory rate limiter keyed by IP.
// Entries are evicted after TTL via Sweep, which is expected to be called
// periodically by a supervised background goroutine.
type IPRateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*limiterEntry
	rate     rate.Limit
	burst    int
	ttl      time.Duration
	clock    clockwork.Clock
}

// NewIPRateLimiter returns a limiter allowing r requests/sec with the given
// burst. An optional ttl may be supplied; when omitted, defaultIPTTL is used.
// The variadic form preserves backwards compatibility with existing two-arg
// call sites while allowing new callers to configure eviction explicitly.
func NewIPRateLimiter(r rate.Limit, burst int, ttl ...time.Duration) *IPRateLimiter {
	t := defaultIPTTL
	if len(ttl) > 0 && ttl[0] > 0 {
		t = ttl[0]
	}
	return &IPRateLimiter{
		limiters: make(map[string]*limiterEntry),
		rate:     r,
		burst:    burst,
		ttl:      t,
		clock:    clockwork.NewRealClock(),
	}
}

// WithClock returns the limiter with the given clock installed. Tests inject
// a fake clock to deterministically advance time.
func (l *IPRateLimiter) WithClock(c clockwork.Clock) *IPRateLimiter {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.clock = c
	return l
}

// GetLimiter returns the *rate.Limiter for ip, creating one on first use and
// refreshing its last-seen timestamp. Preserved for backwards compatibility.
func (l *IPRateLimiter) GetLimiter(ip string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.limiters[ip]
	if !ok {
		e = &limiterEntry{limiter: rate.NewLimiter(l.rate, l.burst)}
		l.limiters[ip] = e
	}
	e.seen = l.clock.Now()
	return e.limiter
}

// Allow returns true iff a request from ip is within the rate budget. It
// refreshes the entry's last-seen timestamp as a side effect.
func (l *IPRateLimiter) Allow(ip string) bool {
	return l.GetLimiter(ip).Allow()
}

// Len returns the number of tracked IP entries. Primarily for tests and
// observability.
func (l *IPRateLimiter) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.limiters)
}

// Sweep removes entries whose last-seen timestamp is older than TTL. Safe to
// call concurrently with Allow/GetLimiter.
func (l *IPRateLimiter) Sweep() {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := l.clock.Now().Add(-l.ttl)
	for k, e := range l.limiters {
		if e.seen.Before(cutoff) {
			delete(l.limiters, k)
		}
	}
}

// CleanupStale is retained for backwards compatibility. It now evicts based
// on TTL rather than the previous (broken) Tokens() == burst heuristic. The
// maxAge argument, when > 0, temporarily overrides the configured TTL for a
// single sweep.
//
// Deprecated: use Sweep instead.
func (l *IPRateLimiter) CleanupStale(maxAge time.Duration) {
	if maxAge <= 0 {
		l.Sweep()
		return
	}
	l.mu.Lock()
	cutoff := l.clock.Now().Add(-maxAge)
	for k, e := range l.limiters {
		if e.seen.Before(cutoff) {
			delete(l.limiters, k)
		}
	}
	l.mu.Unlock()
}

// Limit returns a Gin middleware that rejects with 429 when the caller is
// over budget. Equivalent to the package-level RateLimit helper but scoped to
// an existing limiter instance.
func (l *IPRateLimiter) Limit() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !l.Allow(c.ClientIP()) {
			c.Header("Retry-After", "60")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
			return
		}
		c.Next()
	}
}

// RateLimit returns a Gin middleware backed by a fresh IPRateLimiter with
// the default TTL. Preserved for backwards compatibility with existing
// wiring in cmd/server.
func RateLimit(r rate.Limit, burst int) gin.HandlerFunc {
	return NewIPRateLimiter(r, burst).Limit()
}
