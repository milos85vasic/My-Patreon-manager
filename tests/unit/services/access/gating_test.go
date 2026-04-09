package access_test

import (
	"context"
	"testing"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/services/access"
	"github.com/stretchr/testify/assert"
)

func TestNewTierGater(t *testing.T) {
	gater := access.NewTierGater()
	assert.NotNil(t, gater)
}

func TestTierGater_VerifyAccess_Granted(t *testing.T) {
	gater := access.NewTierGater()
	granted, upgradeURL, err := gater.VerifyAccess(context.Background(), "patron1", "content1", "gold", []string{"gold", "silver"})
	assert.NoError(t, err)
	assert.True(t, granted)
	assert.Empty(t, upgradeURL)
}

func TestTierGater_VerifyAccess_Denied(t *testing.T) {
	gater := access.NewTierGater()
	granted, upgradeURL, err := gater.VerifyAccess(context.Background(), "patron1", "content1", "gold", []string{"bronze"})
	assert.NoError(t, err)
	assert.False(t, granted)
	assert.Contains(t, upgradeURL, "/upgrade")
	assert.Contains(t, upgradeURL, "gold")
}

func TestTierGater_VerifyAccess_EmptyTiers(t *testing.T) {
	gater := access.NewTierGater()
	granted, _, err := gater.VerifyAccess(context.Background(), "patron1", "content1", "gold", nil)
	assert.NoError(t, err)
	assert.False(t, granted)
}

func TestTierGater_CacheHit(t *testing.T) {
	gater := access.NewTierGater()

	granted, _, _ := gater.VerifyAccess(context.Background(), "p1", "c1", "gold", []string{"gold"})
	assert.True(t, granted)

	granted, _, _ = gater.VerifyAccess(context.Background(), "p1", "c1", "gold", []string{})
	assert.True(t, granted)
}

func TestAccessCache_TTLExpiry(t *testing.T) {
	cache := access.NewAccessCache(10 * time.Millisecond)
	cache.Set("key1", true)
	got, ok := cache.Get("key1")
	assert.True(t, ok)
	assert.True(t, got)

	time.Sleep(20 * time.Millisecond)
	got, ok = cache.Get("key1")
	assert.False(t, ok)
}

func TestAccessCache_Invalidate(t *testing.T) {
	cache := access.NewAccessCache(5 * time.Minute)
	cache.Set("key1", true)
	cache.Invalidate("key1")
	_, ok := cache.Get("key1")
	assert.False(t, ok)
}

func TestAccessCache_InvalidateAll(t *testing.T) {
	cache := access.NewAccessCache(5 * time.Minute)
	cache.Set("key1", true)
	cache.Set("key2", false)
	cache.InvalidateAll()
	_, ok := cache.Get("key1")
	assert.False(t, ok)
	_, ok = cache.Get("key2")
	assert.False(t, ok)
}
