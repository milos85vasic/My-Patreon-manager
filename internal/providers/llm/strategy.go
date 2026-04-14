package llm

import (
	"sort"
	"sync"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

// Default scoring weights for content generation model selection.
// Tuned for Patreon content publishing where quality and reliability
// matter more than raw speed.
const (
	DefaultWeightQuality     = 0.40
	DefaultWeightReliability = 0.25
	DefaultWeightCost        = 0.20
	DefaultWeightSpeed       = 0.15
)

// ContentStrategy selects the best LLM model for content generation
// using weighted multi-dimensional scoring. Modeled after the Catalogizer
// strategy pattern with thread-safe operation.
type ContentStrategy struct {
	mu       sync.RWMutex
	weights  map[string]float64
	minScore float64
}

// StrategyOption configures a ContentStrategy at construction time.
type StrategyOption func(*ContentStrategy)

// WithWeights overrides the default scoring weights.
func WithWeights(weights map[string]float64) StrategyOption {
	return func(s *ContentStrategy) {
		s.weights = weights
	}
}

// WithMinScore sets the minimum composite score for a model to be
// considered viable. Models below this threshold are excluded from
// selection results.
func WithMinScore(min float64) StrategyOption {
	return func(s *ContentStrategy) {
		s.minScore = min
	}
}

// ScoredModel pairs a model with its composite score from the strategy.
type ScoredModel struct {
	Model models.ModelInfo `json:"model"`
	Score float64          `json:"score"`
}

// NewContentStrategy creates a strategy with default weights for content
// generation. Use StrategyOption values to customize.
func NewContentStrategy(opts ...StrategyOption) *ContentStrategy {
	s := &ContentStrategy{
		weights: map[string]float64{
			"quality":     DefaultWeightQuality,
			"reliability": DefaultWeightReliability,
			"cost":        DefaultWeightCost,
			"speed":       DefaultWeightSpeed,
		},
		minScore: 0.0,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// ScoreModel computes a composite score for a single model based on its
// attributes and the strategy's weights. The score is in the range [0, 1].
func (s *ContentStrategy) ScoreModel(m models.ModelInfo) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	quality := m.QualityScore // already 0-1

	// Reliability: derive from quality (models with higher quality tend
	// to be more reliable). In a full integration with HealthMonitor the
	// success rate would feed in here; for now we use quality as a proxy.
	reliability := m.QualityScore

	// Cost: invert so lower cost = higher score. Cap at $0.10/1k tokens.
	costScore := 1.0
	if m.CostPer1KTok > 0 {
		costScore = 1.0 - clamp(m.CostPer1KTok/0.10, 0, 1)
	}

	// Speed: invert latency so lower = better. Cap at 5 seconds.
	speedScore := 1.0
	latencyMs := float64(m.LatencyP95.Milliseconds())
	if latencyMs > 0 {
		speedScore = 1.0 - clamp(latencyMs/5000.0, 0, 1)
	}

	composite := quality*s.weights["quality"] +
		reliability*s.weights["reliability"] +
		costScore*s.weights["cost"] +
		speedScore*s.weights["speed"]

	return clamp(composite, 0, 1)
}

// Rank scores all models, filters by minScore, and returns them sorted
// by composite score descending (best first).
func (s *ContentStrategy) Rank(modelList []models.ModelInfo) []ScoredModel {
	scored := make([]ScoredModel, 0, len(modelList))
	for _, m := range modelList {
		sc := s.ScoreModel(m)
		if sc >= s.minScore {
			scored = append(scored, ScoredModel{Model: m, Score: sc})
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})
	return scored
}

// SelectBest returns the highest-scoring model, or nil if no models
// pass the minimum score threshold.
func (s *ContentStrategy) SelectBest(modelList []models.ModelInfo) *ScoredModel {
	ranked := s.Rank(modelList)
	if len(ranked) == 0 {
		return nil
	}
	return &ranked[0]
}

// Weights returns a copy of the current scoring weights.
func (s *ContentStrategy) Weights() map[string]float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make(map[string]float64, len(s.weights))
	for k, v := range s.weights {
		out[k] = v
	}
	return out
}

// SetWeights replaces the scoring weights at runtime (thread-safe).
func (s *ContentStrategy) SetWeights(weights map[string]float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.weights = weights
}

// MinScore returns the current minimum composite score threshold.
func (s *ContentStrategy) MinScore() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.minScore
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
