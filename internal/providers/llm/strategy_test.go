package llm

import (
	"sync"
	"testing"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewContentStrategy_Defaults(t *testing.T) {
	s := NewContentStrategy()
	w := s.Weights()
	assert.Equal(t, DefaultWeightQuality, w["quality"])
	assert.Equal(t, DefaultWeightReliability, w["reliability"])
	assert.Equal(t, DefaultWeightCost, w["cost"])
	assert.Equal(t, DefaultWeightSpeed, w["speed"])
	assert.Equal(t, 0.0, s.MinScore())
}

func TestNewContentStrategy_WithWeights(t *testing.T) {
	custom := map[string]float64{
		"quality":     0.50,
		"reliability": 0.20,
		"cost":        0.10,
		"speed":       0.20,
	}
	s := NewContentStrategy(WithWeights(custom))
	w := s.Weights()
	assert.Equal(t, 0.50, w["quality"])
	assert.Equal(t, 0.20, w["reliability"])
	assert.Equal(t, 0.10, w["cost"])
	assert.Equal(t, 0.20, w["speed"])
}

func TestNewContentStrategy_WithMinScore(t *testing.T) {
	s := NewContentStrategy(WithMinScore(0.6))
	assert.Equal(t, 0.6, s.MinScore())
}

func TestContentStrategy_ScoreModel_HighQuality(t *testing.T) {
	s := NewContentStrategy()
	m := models.ModelInfo{
		ID:           "gpt-4",
		QualityScore: 0.95,
		LatencyP95:   500 * time.Millisecond,
		CostPer1KTok: 0.01,
	}
	score := s.ScoreModel(m)
	assert.Greater(t, score, 0.8)
	assert.LessOrEqual(t, score, 1.0)
}

func TestContentStrategy_ScoreModel_LowQuality(t *testing.T) {
	s := NewContentStrategy()
	m := models.ModelInfo{
		ID:           "cheap-model",
		QualityScore: 0.3,
		LatencyP95:   4 * time.Second,
		CostPer1KTok: 0.08,
	}
	score := s.ScoreModel(m)
	assert.Less(t, score, 0.5)
	assert.GreaterOrEqual(t, score, 0.0)
}

func TestContentStrategy_ScoreModel_ZeroCost(t *testing.T) {
	s := NewContentStrategy()
	m := models.ModelInfo{
		ID:           "free-model",
		QualityScore: 0.7,
		LatencyP95:   1 * time.Second,
		CostPer1KTok: 0.0,
	}
	score := s.ScoreModel(m)
	// zero cost → costScore = 1.0 (maximum)
	assert.Greater(t, score, 0.5)
}

func TestContentStrategy_ScoreModel_ZeroLatency(t *testing.T) {
	s := NewContentStrategy()
	m := models.ModelInfo{
		ID:           "instant-model",
		QualityScore: 0.8,
		LatencyP95:   0,
		CostPer1KTok: 0.02,
	}
	score := s.ScoreModel(m)
	// zero latency → speedScore = 1.0 (maximum)
	assert.Greater(t, score, 0.7)
}

func TestContentStrategy_ScoreModel_VeryExpensive(t *testing.T) {
	s := NewContentStrategy()
	m := models.ModelInfo{
		ID:           "premium",
		QualityScore: 0.99,
		LatencyP95:   100 * time.Millisecond,
		CostPer1KTok: 0.15, // over $0.10 cap
	}
	score := s.ScoreModel(m)
	// cost is clamped, so costScore = 0.0
	// still high due to quality, reliability, speed
	assert.Greater(t, score, 0.6)
}

func TestContentStrategy_ScoreModel_VerySlowLatency(t *testing.T) {
	s := NewContentStrategy()
	m := models.ModelInfo{
		ID:           "slow",
		QualityScore: 0.9,
		LatencyP95:   10 * time.Second, // over 5s cap
		CostPer1KTok: 0.01,
	}
	score := s.ScoreModel(m)
	// speedScore clamped to 0.0
	assert.Greater(t, score, 0.5) // quality + reliability + cost still contribute
}

func TestContentStrategy_Rank(t *testing.T) {
	s := NewContentStrategy()
	modelList := []models.ModelInfo{
		{ID: "low", QualityScore: 0.3, LatencyP95: 2 * time.Second, CostPer1KTok: 0.05},
		{ID: "high", QualityScore: 0.95, LatencyP95: 500 * time.Millisecond, CostPer1KTok: 0.02},
		{ID: "mid", QualityScore: 0.7, LatencyP95: 1 * time.Second, CostPer1KTok: 0.03},
	}

	ranked := s.Rank(modelList)
	require.Len(t, ranked, 3)
	assert.Equal(t, "high", ranked[0].Model.ID)
	assert.Equal(t, "mid", ranked[1].Model.ID)
	assert.Equal(t, "low", ranked[2].Model.ID)
}

func TestContentStrategy_Rank_WithMinScore(t *testing.T) {
	s := NewContentStrategy(WithMinScore(0.5))
	modelList := []models.ModelInfo{
		{ID: "bad", QualityScore: 0.1, LatencyP95: 4 * time.Second, CostPer1KTok: 0.09},
		{ID: "good", QualityScore: 0.9, LatencyP95: 500 * time.Millisecond, CostPer1KTok: 0.02},
	}

	ranked := s.Rank(modelList)
	require.Len(t, ranked, 1)
	assert.Equal(t, "good", ranked[0].Model.ID)
}

func TestContentStrategy_Rank_Empty(t *testing.T) {
	s := NewContentStrategy()
	ranked := s.Rank(nil)
	assert.Empty(t, ranked)
}

func TestContentStrategy_SelectBest(t *testing.T) {
	s := NewContentStrategy()
	modelList := []models.ModelInfo{
		{ID: "low", QualityScore: 0.3, LatencyP95: 2 * time.Second, CostPer1KTok: 0.05},
		{ID: "high", QualityScore: 0.95, LatencyP95: 500 * time.Millisecond, CostPer1KTok: 0.02},
	}

	best := s.SelectBest(modelList)
	require.NotNil(t, best)
	assert.Equal(t, "high", best.Model.ID)
}

func TestContentStrategy_SelectBest_NonePassThreshold(t *testing.T) {
	s := NewContentStrategy(WithMinScore(0.99))
	modelList := []models.ModelInfo{
		{ID: "m1", QualityScore: 0.3, LatencyP95: 4 * time.Second, CostPer1KTok: 0.09},
	}

	best := s.SelectBest(modelList)
	assert.Nil(t, best)
}

func TestContentStrategy_SelectBest_EmptyList(t *testing.T) {
	s := NewContentStrategy()
	best := s.SelectBest(nil)
	assert.Nil(t, best)
}

func TestContentStrategy_SetWeights(t *testing.T) {
	s := NewContentStrategy()
	custom := map[string]float64{
		"quality":     0.60,
		"reliability": 0.10,
		"cost":        0.10,
		"speed":       0.20,
	}
	s.SetWeights(custom)

	w := s.Weights()
	assert.Equal(t, 0.60, w["quality"])
	assert.Equal(t, 0.10, w["reliability"])
}

func TestContentStrategy_Weights_ReturnsCopy(t *testing.T) {
	s := NewContentStrategy()
	w := s.Weights()
	w["quality"] = 999.0 // mutate the copy

	// Original should be unchanged
	assert.Equal(t, DefaultWeightQuality, s.Weights()["quality"])
}

func TestContentStrategy_ConcurrentAccess(t *testing.T) {
	s := NewContentStrategy()
	var wg sync.WaitGroup

	m := models.ModelInfo{
		ID:           "m1",
		QualityScore: 0.8,
		LatencyP95:   1 * time.Second,
		CostPer1KTok: 0.02,
	}

	for i := 0; i < 50; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			s.ScoreModel(m)
		}()
		go func() {
			defer wg.Done()
			s.Weights()
		}()
		go func() {
			defer wg.Done()
			s.SetWeights(map[string]float64{
				"quality": 0.4, "reliability": 0.25, "cost": 0.2, "speed": 0.15,
			})
		}()
	}
	wg.Wait()
}

func TestClamp(t *testing.T) {
	tests := []struct {
		v, lo, hi, want float64
	}{
		{0.5, 0, 1, 0.5},
		{-0.1, 0, 1, 0},
		{1.5, 0, 1, 1},
		{0, 0, 1, 0},
		{1, 0, 1, 1},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, clamp(tt.v, tt.lo, tt.hi))
	}
}

func TestContentStrategy_ScoreModel_Deterministic(t *testing.T) {
	s := NewContentStrategy()
	m := models.ModelInfo{
		ID:           "m1",
		QualityScore: 0.85,
		LatencyP95:   800 * time.Millisecond,
		CostPer1KTok: 0.025,
	}

	score1 := s.ScoreModel(m)
	score2 := s.ScoreModel(m)
	assert.Equal(t, score1, score2)
}

func TestContentStrategy_Rank_StableOrder(t *testing.T) {
	s := NewContentStrategy()
	modelList := []models.ModelInfo{
		{ID: "a", QualityScore: 0.9, LatencyP95: 1 * time.Second, CostPer1KTok: 0.02},
		{ID: "b", QualityScore: 0.5, LatencyP95: 1 * time.Second, CostPer1KTok: 0.02},
		{ID: "c", QualityScore: 0.7, LatencyP95: 1 * time.Second, CostPer1KTok: 0.02},
	}

	ranked := s.Rank(modelList)
	require.Len(t, ranked, 3)
	assert.Equal(t, "a", ranked[0].Model.ID)
	assert.Equal(t, "c", ranked[1].Model.ID)
	assert.Equal(t, "b", ranked[2].Model.ID)
}
