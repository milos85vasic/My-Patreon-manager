package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

type IPRateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	rate     rate.Limit
	burst    int
}

func NewIPRateLimiter(r rate.Limit, burst int) *IPRateLimiter {
	return &IPRateLimiter{
		limiters: make(map[string]*rate.Limiter),
		rate:     r,
		burst:    burst,
	}
}

func (l *IPRateLimiter) GetLimiter(ip string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()
	limiter, exists := l.limiters[ip]
	if !exists {
		limiter = rate.NewLimiter(l.rate, l.burst)
		l.limiters[ip] = limiter
	}
	return limiter
}

func (l *IPRateLimiter) CleanupStale(maxAge time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for ip := range l.limiters {
		if l.limiters[ip].Tokens() == float64(l.burst) {
			delete(l.limiters, ip)
		}
	}
}

func RateLimit(r rate.Limit, burst int) gin.HandlerFunc {
	limiter := NewIPRateLimiter(r, burst)
	return func(c *gin.Context) {
		ip := c.ClientIP()
		if !limiter.GetLimiter(ip).Allow() {
			c.Header("Retry-After", "60")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
			return
		}
		c.Next()
	}
}
