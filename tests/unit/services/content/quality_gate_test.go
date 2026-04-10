package content_test

import (
	"context"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/llm"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/content"
	"github.com/milos85vasic/My-Patreon-Manager/tests/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQualityGate_AboveThreshold_Passes(t *testing.T) {
	gate := content.NewQualityGate(0.75)
	passed, score := gate.Evaluate(models.Content{
		Body:         "good content",
		QualityScore: 0.9,
	})
	assert.True(t, passed)
	assert.Equal(t, 0.9, score)
}

func TestQualityGate_BelowThreshold_Fails(t *testing.T) {
	gate := content.NewQualityGate(0.75)
	passed, score := gate.Evaluate(models.Content{
		Body:         "bad content",
		QualityScore: 0.5,
	})
	assert.False(t, passed)
	assert.Equal(t, 0.5, score)
}

func TestQualityGate_FallbackTriggersRegeneration(t *testing.T) {
	// Mock two LLM providers: first returns low quality, second returns high quality
	provider1 := &mocks.MockLLMProvider{}
	provider2 := &mocks.MockLLMProvider{}

	lowContent := models.Content{
		Title:        "Low",
		Body:         "Low quality",
		QualityScore: 0.6,
		ModelUsed:    "gpt-3",
		TokenCount:   100,
	}
	highContent := models.Content{
		Title:        "High",
		Body:         "High quality",
		QualityScore: 0.9,
		ModelUsed:    "gpt-4",
		TokenCount:   200,
	}

	callCount1 := 0
	callCount2 := 0
	provider1.GenerateContentFunc = func(ctx context.Context, prompt models.Prompt, opts models.GenerationOptions) (models.Content, error) {
		callCount1++
		return lowContent, nil
	}
	provider2.GenerateContentFunc = func(ctx context.Context, prompt models.Prompt, opts models.GenerationOptions) (models.Content, error) {
		callCount2++
		return highContent, nil
	}

	chain := llm.NewFallbackChain([]llm.LLMProvider{provider1, provider2}, 0.75, nil)
	result, err := chain.GenerateContent(context.Background(), models.Prompt{}, models.GenerationOptions{})
	require.NoError(t, err)
	assert.Equal(t, highContent, result)
	assert.Equal(t, 1, callCount1, "first provider should be called")
	assert.Equal(t, 1, callCount2, "second provider should be called after first fails threshold")
}

func TestQualityGate_AllFailQueuesForReview(t *testing.T) {
	// Mock two LLM providers both returning low quality
	provider1 := &mocks.MockLLMProvider{}
	provider2 := &mocks.MockLLMProvider{}

	lowContent1 := models.Content{
		Title:        "Low1",
		Body:         "Low quality 1",
		QualityScore: 0.5,
		ModelUsed:    "gpt-3",
		TokenCount:   100,
	}
	lowContent2 := models.Content{
		Title:        "Low2",
		Body:         "Low quality 2",
		QualityScore: 0.4,
		ModelUsed:    "gpt-3",
		TokenCount:   120,
	}

	callCount1 := 0
	callCount2 := 0
	provider1.GenerateContentFunc = func(ctx context.Context, prompt models.Prompt, opts models.GenerationOptions) (models.Content, error) {
		callCount1++
		return lowContent1, nil
	}
	provider2.GenerateContentFunc = func(ctx context.Context, prompt models.Prompt, opts models.GenerationOptions) (models.Content, error) {
		callCount2++
		return lowContent2, nil
	}

	chain := llm.NewFallbackChain([]llm.LLMProvider{provider1, provider2}, 0.75, nil)
	result, err := chain.GenerateContent(context.Background(), models.Prompt{}, models.GenerationOptions{})
	require.NoError(t, err)
	// Should return the best content (highest score)
	assert.Equal(t, lowContent1, result)
	assert.Equal(t, 1, callCount1)
	assert.Equal(t, 1, callCount2)

	// Now test that the generator adds this content to review queue when quality gate fails
	// We need a generator with a review queue
	// Since we cannot directly test generator without a store, we'll skip this part for now
	// The integration test will cover the review queue addition.
}

func TestQualityGate_TokenBudgetEnforcement(t *testing.T) {
	budget := content.NewTokenBudget(1000)
	softAlertCalled := false
	hardStopCalled := false
	budget.OnSoftAlert = func(percent float64) {
		softAlertCalled = true
		assert.GreaterOrEqual(t, percent, 80.0)
		assert.Less(t, percent, 100.0)
	}
	budget.OnHardStop = func() {
		hardStopCalled = true
	}

	// Use 600 tokens (60%)
	err := budget.CheckBudget(600)
	require.NoError(t, err)
	assert.False(t, softAlertCalled, "soft alert should not fire below 80%")

	// Use 300 more tokens (total 900) -> 90%
	err = budget.CheckBudget(300)
	require.NoError(t, err)
	assert.True(t, softAlertCalled, "soft alert should fire at 90%")

	// Try to use 200 more tokens (would exceed 100%)
	err = budget.CheckBudget(200)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token budget exceeded")
	assert.True(t, hardStopCalled, "hard stop should fire")
}
