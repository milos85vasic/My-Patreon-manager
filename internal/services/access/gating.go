package access

import (
	"context"
	"sync"
	"time"
)

type AccessCache struct {
	mu    sync.RWMutex
	cache map[string]cacheEntry
	ttl   time.Duration
}

type cacheEntry struct {
	hasAccess bool
	updatedAt time.Time
}

func NewAccessCache(ttl time.Duration) *AccessCache {
	return &AccessCache{
		cache: make(map[string]cacheEntry),
		ttl:   ttl,
	}
}

func (c *AccessCache) Get(key string) (bool, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.cache[key]
	if !ok || time.Since(entry.updatedAt) > c.ttl {
		return false, false
	}
	return entry.hasAccess, true
}

func (c *AccessCache) Set(key string, hasAccess bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[key] = cacheEntry{hasAccess: hasAccess, updatedAt: time.Now()}
}

func (c *AccessCache) Invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.cache, key)
}

func (c *AccessCache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache = make(map[string]cacheEntry)
}

type TierGater struct {
	cache *AccessCache
}

func NewTierGater() *TierGater {
	return &TierGater{
		cache: NewAccessCache(5 * time.Minute),
	}
}

func (g *TierGater) VerifyAccess(ctx context.Context, patronID, contentID, requiredTier string, patronTiers []string) (bool, string, error) {
	cacheKey := patronID + ":" + contentID + ":" + requiredTier
	if cached, ok := g.cache.Get(cacheKey); ok {
		upgradeURL := ""
		if !cached {
			upgradeURL = "/upgrade?content=" + contentID + "&required_tier=" + requiredTier
		}
		return cached, upgradeURL, nil
	}

	for _, tier := range patronTiers {
		if tier == requiredTier {
			g.cache.Set(cacheKey, true)
			return true, "", nil
		}
	}

	g.cache.Set(cacheKey, false)
	upgradeURL := "/upgrade?content=" + contentID + "&required_tier=" + requiredTier
	return false, upgradeURL, nil
}
