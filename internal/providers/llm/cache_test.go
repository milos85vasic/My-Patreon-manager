package llm

import (
	"sync"
	"testing"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewScoreCache(t *testing.T) {
	c := NewScoreCache(5 * time.Minute)
	assert.NotNil(t, c)
	assert.Equal(t, 5*time.Minute, c.ttl)
}

func TestScoreCache_SetAndGetScore(t *testing.T) {
	c := NewScoreCache(5 * time.Minute)
	c.SetScore("model-1", 0.92)

	score, hit := c.GetScore("model-1")
	assert.True(t, hit)
	assert.Equal(t, 0.92, score)
}

func TestScoreCache_GetScore_Miss(t *testing.T) {
	c := NewScoreCache(5 * time.Minute)

	score, hit := c.GetScore("nonexistent")
	assert.False(t, hit)
	assert.Equal(t, 0.0, score)
}

func TestScoreCache_GetScore_Expired(t *testing.T) {
	c := NewScoreCache(1 * time.Millisecond)
	c.SetScore("model-1", 0.85)
	time.Sleep(5 * time.Millisecond)

	score, hit := c.GetScore("model-1")
	assert.False(t, hit)
	assert.Equal(t, 0.0, score)
}

func TestScoreCache_SetAndGetModels(t *testing.T) {
	c := NewScoreCache(5 * time.Minute)
	modelList := []models.ModelInfo{
		{ID: "m1", Name: "Model 1", QualityScore: 0.9},
		{ID: "m2", Name: "Model 2", QualityScore: 0.8},
	}
	c.SetModels(modelList)

	result, hit := c.GetModels()
	assert.True(t, hit)
	require.Len(t, result, 2)
	assert.Equal(t, "m1", result[0].ID)
	assert.Equal(t, "m2", result[1].ID)
}

func TestScoreCache_GetModels_Miss(t *testing.T) {
	c := NewScoreCache(5 * time.Minute)

	result, hit := c.GetModels()
	assert.False(t, hit)
	assert.Nil(t, result)
}

func TestScoreCache_GetModels_Expired(t *testing.T) {
	c := NewScoreCache(1 * time.Millisecond)
	c.SetModels([]models.ModelInfo{{ID: "m1"}})
	time.Sleep(5 * time.Millisecond)

	result, hit := c.GetModels()
	assert.False(t, hit)
	assert.Nil(t, result)
}

func TestScoreCache_Invalidate(t *testing.T) {
	c := NewScoreCache(5 * time.Minute)
	c.SetScore("m1", 0.9)
	c.SetScore("m2", 0.8)

	c.Invalidate("m1")

	_, hit := c.GetScore("m1")
	assert.False(t, hit)

	score, hit := c.GetScore("m2")
	assert.True(t, hit)
	assert.Equal(t, 0.8, score)
}

func TestScoreCache_InvalidateAll(t *testing.T) {
	c := NewScoreCache(5 * time.Minute)
	c.SetScore("m1", 0.9)
	c.SetModels([]models.ModelInfo{{ID: "m1"}})

	c.InvalidateAll()

	_, hit := c.GetScore("m1")
	assert.False(t, hit)

	_, hit = c.GetModels()
	assert.False(t, hit)
}

func TestScoreCache_Stats(t *testing.T) {
	c := NewScoreCache(5 * time.Minute)
	c.SetScore("m1", 0.9)

	c.GetScore("m1")          // hit
	c.GetScore("nonexistent") // miss
	c.GetModels()             // miss (no models cached)

	hits, misses := c.Stats()
	assert.Equal(t, int64(1), hits)
	assert.Equal(t, int64(2), misses)
}

func TestScoreCache_ZeroTTL_DisablesCaching(t *testing.T) {
	c := NewScoreCache(0)

	c.SetScore("m1", 0.9)
	score, hit := c.GetScore("m1")
	assert.False(t, hit)
	assert.Equal(t, 0.0, score)

	c.SetModels([]models.ModelInfo{{ID: "m1"}})
	result, hit := c.GetModels()
	assert.False(t, hit)
	assert.Nil(t, result)

	hits, misses := c.Stats()
	assert.Equal(t, int64(0), hits)
	assert.Equal(t, int64(2), misses)
}

func TestScoreCache_NegativeTTL_DisablesCaching(t *testing.T) {
	c := NewScoreCache(-1 * time.Minute)

	c.SetScore("m1", 0.5)
	_, hit := c.GetScore("m1")
	assert.False(t, hit)

	c.SetModels([]models.ModelInfo{{ID: "m1"}})
	_, hit = c.GetModels()
	assert.False(t, hit)
}

func TestScoreCache_ConcurrentAccess(t *testing.T) {
	c := NewScoreCache(5 * time.Minute)
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(id int) {
			defer wg.Done()
			c.SetScore("model", float64(id)/100.0)
		}(i)
		go func() {
			defer wg.Done()
			c.GetScore("model")
		}()
	}
	wg.Wait()

	// Should not panic; final state is valid
	_, hit := c.GetScore("model")
	assert.True(t, hit)
}

func TestScoreCache_OverwriteScore(t *testing.T) {
	c := NewScoreCache(5 * time.Minute)
	c.SetScore("m1", 0.5)
	c.SetScore("m1", 0.9)

	score, hit := c.GetScore("m1")
	assert.True(t, hit)
	assert.Equal(t, 0.9, score)
}

func TestScoreCache_OverwriteModels(t *testing.T) {
	c := NewScoreCache(5 * time.Minute)
	c.SetModels([]models.ModelInfo{{ID: "old"}})
	c.SetModels([]models.ModelInfo{{ID: "new"}})

	result, hit := c.GetModels()
	assert.True(t, hit)
	require.Len(t, result, 1)
	assert.Equal(t, "new", result[0].ID)
}
