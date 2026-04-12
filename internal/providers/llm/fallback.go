package llm

import (
	"context"
	"fmt"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/concurrency"
	"github.com/milos85vasic/My-Patreon-Manager/internal/metrics"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

type FallbackChain struct {
	providers []LLMProvider
	breakers  []*metrics.CircuitBreaker
	threshold float64
	metrics   metrics.MetricsCollector
	sem       *concurrency.Semaphore
}

// Option configures a FallbackChain at construction time.
type Option func(*FallbackChain)

// WithSemaphore bounds the total number of in-flight GenerateContent calls
// across every provider in the chain. A nil semaphore (the default) disables
// the cap entirely.
func WithSemaphore(s *concurrency.Semaphore) Option {
	return func(fc *FallbackChain) { fc.sem = s }
}

// WithBreakerTimeouts overrides the circuit-breaker timeout and half-open
// timeout for every breaker in the chain. Intended for tests that need fast
// breaker resets without waiting 60 seconds.
func WithBreakerTimeouts(timeout, halfOpen time.Duration) Option {
	return func(fc *FallbackChain) {
		for i := range fc.providers {
			fc.breakers[i] = metrics.NewCircuitBreaker(
				fmt.Sprintf("llm_fallback_%d", i),
				3, timeout, halfOpen,
				metrics.DefaultOnTrip, metrics.DefaultOnReset,
			)
		}
	}
}

func NewFallbackChain(providers []LLMProvider, threshold float64, m metrics.MetricsCollector, opts ...Option) *FallbackChain {
	breakers := make([]*metrics.CircuitBreaker, len(providers))
	for i := range providers {
		breakers[i] = metrics.NewCircuitBreaker(
			fmt.Sprintf("llm_fallback_%d", i),
			3, 60*time.Second, 30*time.Second,
			metrics.DefaultOnTrip, metrics.DefaultOnReset,
		)
	}
	fc := &FallbackChain{
		providers: providers,
		breakers:  breakers,
		threshold: threshold,
		metrics:   m,
	}
	for _, o := range opts {
		o(fc)
	}
	return fc
}

func (fc *FallbackChain) GenerateContent(ctx context.Context, prompt models.Prompt, opts models.GenerationOptions) (models.Content, error) {
	if fc.sem != nil {
		if err := fc.sem.Acquire(ctx, 1); err != nil {
			return models.Content{}, err
		}
		defer fc.sem.Release(1)
	}

	var lastErr error
	var bestContent models.Content
	bestScore := -1.0

	for i, provider := range fc.providers {
		if fc.breakers[i].State() == metrics.CircuitOpen {
			continue
		}

		result, err := fc.breakers[i].Execute(func() (interface{}, error) {
			return provider.GenerateContent(ctx, prompt, opts)
		})
		if err != nil {
			lastErr = err
			continue
		}

		content := result.(models.Content)
		if content.QualityScore >= fc.threshold {
			return content, nil
		}

		if fc.metrics != nil {
			fc.metrics.RecordLLMQualityScore("", content.QualityScore)
		}
		// store the best content so far (highest score)
		if content.QualityScore > bestScore {
			bestScore = content.QualityScore
			bestContent = content
		}
		lastErr = fmt.Errorf("quality score %.2f below threshold %.2f for model %s",
			content.QualityScore, fc.threshold, content.ModelUsed)
	}

	// If we have at least one content (even below threshold), return it
	if bestScore >= 0 {
		return bestContent, nil
	}

	return models.Content{}, fmt.Errorf("all providers failed: %w", lastErr)
}

func (fc *FallbackChain) GetAvailableModels(ctx context.Context) ([]models.ModelInfo, error) {
	for i, provider := range fc.providers {
		if fc.breakers[i].State() == metrics.CircuitOpen {
			continue
		}
		models, err := provider.GetAvailableModels(ctx)
		if err == nil {
			return models, nil
		}
	}
	return nil, fmt.Errorf("all providers failed to list models")
}

func (fc *FallbackChain) GetModelQualityScore(ctx context.Context, modelID string) (float64, error) {
	for i, provider := range fc.providers {
		if fc.breakers[i].State() == metrics.CircuitOpen {
			continue
		}
		score, err := provider.GetModelQualityScore(ctx, modelID)
		if err == nil {
			return score, nil
		}
	}
	return 0, fmt.Errorf("all providers failed to get quality score")
}

func (fc *FallbackChain) GetTokenUsage(ctx context.Context) (models.UsageStats, error) {
	if len(fc.providers) == 0 {
		return models.UsageStats{}, fmt.Errorf("no providers available")
	}
	return fc.providers[0].GetTokenUsage(ctx)
}
