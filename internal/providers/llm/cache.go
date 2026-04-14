package llm

import (
	"sync"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

// ScoreCache is a thread-safe, TTL-based cache for model quality scores
// and model lists. Modeled after the Catalogizer cachedScore pattern with
// RWMutex protection.
type ScoreCache struct {
	mu     sync.RWMutex
	scores map[string]cachedScore
	models *cachedModels
	ttl    time.Duration
	hits   int64
	misses int64
}

type cachedScore struct {
	score     float64
	expiresAt time.Time
}

type cachedModels struct {
	models    []models.ModelInfo
	expiresAt time.Time
}

// NewScoreCache creates a cache with the given TTL. A zero or negative TTL
// disables caching (every lookup is a miss).
func NewScoreCache(ttl time.Duration) *ScoreCache {
	return &ScoreCache{
		scores: make(map[string]cachedScore),
		ttl:    ttl,
	}
}

// GetScore returns a cached quality score for the model ID, along with a
// hit flag. Returns (0, false) on miss or expiry.
func (c *ScoreCache) GetScore(modelID string) (float64, bool) {
	if c.ttl <= 0 {
		c.mu.Lock()
		c.misses++
		c.mu.Unlock()
		return 0, false
	}

	c.mu.RLock()
	entry, ok := c.scores[modelID]
	c.mu.RUnlock()

	if !ok || time.Now().After(entry.expiresAt) {
		c.mu.Lock()
		c.misses++
		c.mu.Unlock()
		return 0, false
	}

	c.mu.Lock()
	c.hits++
	c.mu.Unlock()
	return entry.score, true
}

// SetScore stores a quality score for the model ID with the configured TTL.
func (c *ScoreCache) SetScore(modelID string, score float64) {
	if c.ttl <= 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.scores[modelID] = cachedScore{
		score:     score,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// GetModels returns the cached model list if it exists and has not expired.
func (c *ScoreCache) GetModels() ([]models.ModelInfo, bool) {
	if c.ttl <= 0 {
		c.mu.Lock()
		c.misses++
		c.mu.Unlock()
		return nil, false
	}

	c.mu.RLock()
	m := c.models
	c.mu.RUnlock()

	if m == nil || time.Now().After(m.expiresAt) {
		c.mu.Lock()
		c.misses++
		c.mu.Unlock()
		return nil, false
	}

	c.mu.Lock()
	c.hits++
	c.mu.Unlock()
	return m.models, true
}

// SetModels stores a model list with the configured TTL.
func (c *ScoreCache) SetModels(modelList []models.ModelInfo) {
	if c.ttl <= 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.models = &cachedModels{
		models:    modelList,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// Invalidate removes the cached score for a single model ID.
func (c *ScoreCache) Invalidate(modelID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.scores, modelID)
}

// InvalidateAll clears the entire cache (scores and models).
func (c *ScoreCache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.scores = make(map[string]cachedScore)
	c.models = nil
}

// Stats returns cache hit/miss counters.
func (c *ScoreCache) Stats() (hits, misses int64) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.hits, c.misses
}
