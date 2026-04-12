package llm_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/llm"
	"github.com/milos85vasic/My-Patreon-Manager/tests/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFallbackChain_GenerateContent_SuccessFirstProvider(t *testing.T) {
	mockMetrics := &mockMetricsCollector{}
	provider1 := &mocks.MockLLMProvider{}
	provider2 := &mocks.MockLLMProvider{}

	content := models.Content{
		Title:        "Test Title",
		Body:         "Test Body",
		QualityScore: 0.9,
		ModelUsed:    "gpt-4",
		TokenCount:   100,
	}

	provider1.GenerateContentFunc = func(ctx context.Context, prompt models.Prompt, opts models.GenerationOptions) (models.Content, error) {
		return content, nil
	}
	provider2.GenerateContentFunc = func(ctx context.Context, prompt models.Prompt, opts models.GenerationOptions) (models.Content, error) {
		t.Error("Second provider should not be called")
		return models.Content{}, errors.New("unexpected call")
	}

	chain := llm.NewFallbackChain([]llm.LLMProvider{provider1, provider2}, 0.8, mockMetrics)
	prompt := models.Prompt{}
	opts := models.GenerationOptions{}
	result, err := chain.GenerateContent(context.Background(), prompt, opts)
	require.NoError(t, err)
	assert.Equal(t, content, result)
	assert.Len(t, mockMetrics.recordedLLMQuality, 0, "No quality metric should be recorded when score meets threshold")
}

func TestFallbackChain_GenerateContent_QualityThresholdFallback(t *testing.T) {
	mockMetrics := &mockMetricsCollector{}
	provider1 := &mocks.MockLLMProvider{}
	provider2 := &mocks.MockLLMProvider{}

	lowQuality := models.Content{
		Title:        "Low Quality",
		Body:         "Low",
		QualityScore: 0.7,
		ModelUsed:    "gpt-3",
		TokenCount:   50,
	}
	highQuality := models.Content{
		Title:        "High Quality",
		Body:         "High",
		QualityScore: 0.9,
		ModelUsed:    "gpt-4",
		TokenCount:   100,
	}

	calls := 0
	provider1.GenerateContentFunc = func(ctx context.Context, prompt models.Prompt, opts models.GenerationOptions) (models.Content, error) {
		calls++
		return lowQuality, nil
	}
	provider2.GenerateContentFunc = func(ctx context.Context, prompt models.Prompt, opts models.GenerationOptions) (models.Content, error) {
		calls++
		return highQuality, nil
	}

	chain := llm.NewFallbackChain([]llm.LLMProvider{provider1, provider2}, 0.8, mockMetrics)
	prompt := models.Prompt{}
	opts := models.GenerationOptions{}
	result, err := chain.GenerateContent(context.Background(), prompt, opts)
	require.NoError(t, err)
	assert.Equal(t, highQuality, result)
	assert.Equal(t, 2, calls, "Both providers should be called")
	require.Len(t, mockMetrics.recordedLLMQuality, 1)
	assert.Equal(t, "", mockMetrics.recordedLLMQuality[0].repository)
	assert.Equal(t, 0.7, mockMetrics.recordedLLMQuality[0].score)
}

func TestFallbackChain_GenerateContent_CircuitBreakerOpen(t *testing.T) {
	mockMetrics := &mockMetricsCollector{}
	_ = mockMetrics // use to avoid unused warning
	provider1 := &mocks.MockLLMProvider{}
	provider2 := &mocks.MockLLMProvider{}

	provider1Calls := 0
	provider2Calls := 0
	provider1.GenerateContentFunc = func(ctx context.Context, prompt models.Prompt, opts models.GenerationOptions) (models.Content, error) {
		provider1Calls++
		return models.Content{}, errors.New("failure")
	}
	provider2.GenerateContentFunc = func(ctx context.Context, prompt models.Prompt, opts models.GenerationOptions) (models.Content, error) {
		provider2Calls++
		return models.Content{
			Title:        "Success",
			Body:         "Body",
			QualityScore: 0.9,
			ModelUsed:    "gpt-4",
			TokenCount:   100,
		}, nil
	}

	chain := llm.NewFallbackChain([]llm.LLMProvider{provider1, provider2}, 0.8, mockMetrics)
	// First three calls: provider1 fails, provider2 succeeds, breaker counts failures
	for i := 0; i < 3; i++ {
		result, err := chain.GenerateContent(context.Background(), models.Prompt{}, models.GenerationOptions{})
		require.NoError(t, err)
		assert.Equal(t, "Success", result.Title)
	}
	assert.Equal(t, 3, provider1Calls, "Provider1 should be called 3 times before breaker opens")
	assert.Equal(t, 3, provider2Calls, "Provider2 should be called 3 times as fallback")

	// After three consecutive failures, breaker should be open for provider1
	// Next call should skip provider1 (call count unchanged) and use provider2
	provider1CallsBefore := provider1Calls
	provider2CallsBefore := provider2Calls
	result, err := chain.GenerateContent(context.Background(), models.Prompt{}, models.GenerationOptions{})
	require.NoError(t, err)
	assert.Equal(t, "Success", result.Title)
	assert.Equal(t, provider1CallsBefore, provider1Calls, "Provider1 should not be called when breaker is open")
	assert.Equal(t, provider2CallsBefore+1, provider2Calls, "Provider2 should be called again")
}

func TestFallbackChain_GenerateContent_AllProvidersFail(t *testing.T) {
	mockMetrics := &mockMetricsCollector{}
	_ = mockMetrics
	provider1 := &mocks.MockLLMProvider{}
	provider2 := &mocks.MockLLMProvider{}

	provider1.GenerateContentFunc = func(ctx context.Context, prompt models.Prompt, opts models.GenerationOptions) (models.Content, error) {
		return models.Content{}, errors.New("provider1 error")
	}
	provider2.GenerateContentFunc = func(ctx context.Context, prompt models.Prompt, opts models.GenerationOptions) (models.Content, error) {
		return models.Content{}, errors.New("provider2 error")
	}

	chain := llm.NewFallbackChain([]llm.LLMProvider{provider1, provider2}, 0.8, mockMetrics)
	prompt := models.Prompt{}
	opts := models.GenerationOptions{}
	_, err := chain.GenerateContent(context.Background(), prompt, opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "all providers failed")
	assert.Contains(t, err.Error(), "provider2 error")
}

func TestFallbackChain_GenerateContent_CircuitBreakerTripAfterThreshold(t *testing.T) {
	mockMetrics := &mockMetricsCollector{}
	_ = mockMetrics
	provider1 := &mocks.MockLLMProvider{}
	provider2 := &mocks.MockLLMProvider{}

	provider1Calls := 0
	provider2Calls := 0
	provider1.GenerateContentFunc = func(ctx context.Context, prompt models.Prompt, opts models.GenerationOptions) (models.Content, error) {
		provider1Calls++
		return models.Content{}, errors.New("failure")
	}
	provider2.GenerateContentFunc = func(ctx context.Context, prompt models.Prompt, opts models.GenerationOptions) (models.Content, error) {
		provider2Calls++
		return models.Content{
			Title:        "Fallback",
			Body:         "Body",
			QualityScore: 0.9,
			ModelUsed:    "gpt-4",
			TokenCount:   100,
		}, nil
	}

	chain := llm.NewFallbackChain([]llm.LLMProvider{provider1, provider2}, 0.8, mockMetrics)
	// First three calls: provider1 fails, provider2 succeeds, breaker counts failures
	for i := 0; i < 3; i++ {
		result, err := chain.GenerateContent(context.Background(), models.Prompt{}, models.GenerationOptions{})
		require.NoError(t, err)
		assert.Equal(t, "Fallback", result.Title)
	}
	assert.Equal(t, 3, provider1Calls, "Provider1 should be called 3 times before breaker opens")
	assert.Equal(t, 3, provider2Calls, "Provider2 should be called 3 times as fallback")

	// After three consecutive failures, breaker should be open for provider1
	// Next call should skip provider1 (call count unchanged) and use provider2
	provider1CallsBefore := provider1Calls
	provider2CallsBefore := provider2Calls
	result, err := chain.GenerateContent(context.Background(), models.Prompt{}, models.GenerationOptions{})
	require.NoError(t, err)
	assert.Equal(t, "Fallback", result.Title)
	assert.Equal(t, provider1CallsBefore, provider1Calls, "Provider1 should not be called when breaker is open")
	assert.Equal(t, provider2CallsBefore+1, provider2Calls, "Provider2 should be called again")
}

func TestFallbackChain_GenerateContent_CircuitBreakerResetAfterCooldown(t *testing.T) {
	mockMetrics := &mockMetricsCollector{}
	_ = mockMetrics
	provider1 := &mocks.MockLLMProvider{}
	provider2 := &mocks.MockLLMProvider{}

	callCount := 0
	provider1.GenerateContentFunc = func(ctx context.Context, prompt models.Prompt, opts models.GenerationOptions) (models.Content, error) {
		callCount++
		if callCount <= 3 {
			return models.Content{}, errors.New("failure")
		}
		// After cooldown, succeed
		return models.Content{
			Title:        "Success after reset",
			Body:         "Body",
			QualityScore: 0.9,
			ModelUsed:    "gpt-4",
			TokenCount:   100,
		}, nil
	}
	provider2.GenerateContentFunc = func(ctx context.Context, prompt models.Prompt, opts models.GenerationOptions) (models.Content, error) {
		return models.Content{}, errors.New("provider2 error")
	}

	// Use short breaker timeouts so reset happens quickly in tests.
	chain := llm.NewFallbackChain(
		[]llm.LLMProvider{provider1, provider2}, 0.8, mockMetrics,
		llm.WithBreakerTimeouts(100*time.Millisecond, 50*time.Millisecond),
	)

	// Trip the breaker: 3 failures from provider1
	for i := 0; i < 3; i++ {
		chain.GenerateContent(context.Background(), models.Prompt{}, models.GenerationOptions{})
	}

	// Breaker is open: provider1 is skipped, provider2 fails => all fail
	_, err := chain.GenerateContent(context.Background(), models.Prompt{}, models.GenerationOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "all providers failed")

	// Wait for breaker timeout to transition to half-open
	time.Sleep(150 * time.Millisecond)

	// Provider1 should now be called again (half-open), and callCount > 3 so it succeeds
	result, err := chain.GenerateContent(context.Background(), models.Prompt{}, models.GenerationOptions{})
	require.NoError(t, err)
	assert.Equal(t, "Success after reset", result.Title)
}

func TestFallbackChain_GetAvailableModels_Fallback(t *testing.T) {
	mockMetrics := &mockMetricsCollector{}
	_ = mockMetrics
	provider1 := &mocks.MockLLMProvider{}
	provider2 := &mocks.MockLLMProvider{}

	models2 := []models.ModelInfo{
		{ID: "gpt-4", Name: "GPT-4", QualityScore: 0.95},
	}

	provider1.GetAvailableModelsFunc = func(ctx context.Context) ([]models.ModelInfo, error) {
		return nil, errors.New("provider1 error")
	}
	provider2.GetAvailableModelsFunc = func(ctx context.Context) ([]models.ModelInfo, error) {
		return models2, nil
	}

	chain := llm.NewFallbackChain([]llm.LLMProvider{provider1, provider2}, 0.8, mockMetrics)
	result, err := chain.GetAvailableModels(context.Background())
	require.NoError(t, err)
	assert.Equal(t, models2, result)
}

func TestFallbackChain_GetModelQualityScore_Fallback(t *testing.T) {
	mockMetrics := &mockMetricsCollector{}
	_ = mockMetrics
	provider1 := &mocks.MockLLMProvider{}
	provider2 := &mocks.MockLLMProvider{}

	provider1.GetModelQualityScoreFunc = func(ctx context.Context, modelID string) (float64, error) {
		return 0, errors.New("provider1 error")
	}
	provider2.GetModelQualityScoreFunc = func(ctx context.Context, modelID string) (float64, error) {
		return 0.92, nil
	}

	chain := llm.NewFallbackChain([]llm.LLMProvider{provider1, provider2}, 0.8, mockMetrics)
	score, err := chain.GetModelQualityScore(context.Background(), "gpt-4")
	require.NoError(t, err)
	assert.Equal(t, 0.92, score)
}

func TestFallbackChain_GetAvailableModels_AllProvidersFail(t *testing.T) {
	mockMetrics := &mockMetricsCollector{}
	_ = mockMetrics
	provider1 := &mocks.MockLLMProvider{}
	provider2 := &mocks.MockLLMProvider{}

	provider1.GetAvailableModelsFunc = func(ctx context.Context) ([]models.ModelInfo, error) {
		return nil, errors.New("provider1 error")
	}
	provider2.GetAvailableModelsFunc = func(ctx context.Context) ([]models.ModelInfo, error) {
		return nil, errors.New("provider2 error")
	}

	chain := llm.NewFallbackChain([]llm.LLMProvider{provider1, provider2}, 0.8, mockMetrics)
	_, err := chain.GetAvailableModels(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "all providers failed to list models")
}

func TestFallbackChain_GetAvailableModels_CircuitBreakerOpen(t *testing.T) {
	mockMetrics := &mockMetricsCollector{}
	_ = mockMetrics
	provider1 := &mocks.MockLLMProvider{}
	provider2 := &mocks.MockLLMProvider{}

	provider1.GetAvailableModelsFunc = func(ctx context.Context) ([]models.ModelInfo, error) {
		return nil, errors.New("provider1 error")
	}
	models2 := []models.ModelInfo{
		{ID: "gpt-4", Name: "GPT-4", QualityScore: 0.95},
	}
	provider2.GetAvailableModelsFunc = func(ctx context.Context) ([]models.ModelInfo, error) {
		return models2, nil
	}

	chain := llm.NewFallbackChain([]llm.LLMProvider{provider1, provider2}, 0.8, mockMetrics)
	// Trip breaker for provider1 by causing failures
	for i := 0; i < 3; i++ {
		_, _ = chain.GetAvailableModels(context.Background())
	}
	// Now provider1 breaker should be open, provider2 should be used
	result, err := chain.GetAvailableModels(context.Background())
	require.NoError(t, err)
	assert.Equal(t, models2, result)
}

func TestFallbackChain_GetModelQualityScore_AllProvidersFail(t *testing.T) {
	mockMetrics := &mockMetricsCollector{}
	_ = mockMetrics
	provider1 := &mocks.MockLLMProvider{}
	provider2 := &mocks.MockLLMProvider{}

	provider1.GetModelQualityScoreFunc = func(ctx context.Context, modelID string) (float64, error) {
		return 0, errors.New("provider1 error")
	}
	provider2.GetModelQualityScoreFunc = func(ctx context.Context, modelID string) (float64, error) {
		return 0, errors.New("provider2 error")
	}

	chain := llm.NewFallbackChain([]llm.LLMProvider{provider1, provider2}, 0.8, mockMetrics)
	_, err := chain.GetModelQualityScore(context.Background(), "gpt-4")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "all providers failed to get quality score")
}

func TestFallbackChain_GetModelQualityScore_CircuitBreakerOpen(t *testing.T) {
	mockMetrics := &mockMetricsCollector{}
	_ = mockMetrics
	provider1 := &mocks.MockLLMProvider{}
	provider2 := &mocks.MockLLMProvider{}

	provider1.GetModelQualityScoreFunc = func(ctx context.Context, modelID string) (float64, error) {
		return 0, errors.New("provider1 error")
	}
	provider2.GetModelQualityScoreFunc = func(ctx context.Context, modelID string) (float64, error) {
		return 0.92, nil
	}

	chain := llm.NewFallbackChain([]llm.LLMProvider{provider1, provider2}, 0.8, mockMetrics)
	// Trip breaker for provider1 by causing failures
	for i := 0; i < 3; i++ {
		_, _ = chain.GetModelQualityScore(context.Background(), "gpt-4")
	}
	// Now provider1 breaker should be open, provider2 should be used
	score, err := chain.GetModelQualityScore(context.Background(), "gpt-4")
	require.NoError(t, err)
	assert.Equal(t, 0.92, score)
}

func TestFallbackChain_GetTokenUsage_Error(t *testing.T) {
	mockMetrics := &mockMetricsCollector{}
	_ = mockMetrics
	provider1 := &mocks.MockLLMProvider{}

	provider1.GetTokenUsageFunc = func(ctx context.Context) (models.UsageStats, error) {
		return models.UsageStats{}, errors.New("token usage error")
	}

	chain := llm.NewFallbackChain([]llm.LLMProvider{provider1}, 0.8, mockMetrics)
	_, err := chain.GetTokenUsage(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token usage error")
}

func TestFallbackChain_GetTokenUsage_FirstProvider(t *testing.T) {
	mockMetrics := &mockMetricsCollector{}
	_ = mockMetrics
	provider1 := &mocks.MockLLMProvider{}
	provider2 := &mocks.MockLLMProvider{}

	usage := models.UsageStats{
		TotalTokens:   1000,
		EstimatedCost: 0.05,
		BudgetLimit:   100.0,
		BudgetUsedPct: 0.05,
	}
	provider1.GetTokenUsageFunc = func(ctx context.Context) (models.UsageStats, error) {
		return usage, nil
	}
	provider2.GetTokenUsageFunc = func(ctx context.Context) (models.UsageStats, error) {
		return models.UsageStats{}, errors.New("should not be called")
	}

	chain := llm.NewFallbackChain([]llm.LLMProvider{provider1, provider2}, 0.8, mockMetrics)
	result, err := chain.GetTokenUsage(context.Background())
	require.NoError(t, err)
	assert.Equal(t, usage, result)
}

func TestFallbackChain_GetTokenUsage_NoProviders(t *testing.T) {
	mockMetrics := &mockMetricsCollector{}
	_ = mockMetrics
	chain := llm.NewFallbackChain([]llm.LLMProvider{}, 0.8, mockMetrics)
	_, err := chain.GetTokenUsage(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no providers available")
}

func TestFallbackChain_CircuitBreakerOpen_SkipsProviderForAllMethods(t *testing.T) {
	mockMetrics := &mockMetricsCollector{}
	_ = mockMetrics
	provider1 := &mocks.MockLLMProvider{}
	provider2 := &mocks.MockLLMProvider{}

	// Setup provider1 to fail on GenerateContent (to trip breaker)
	provider1.GenerateContentFunc = func(ctx context.Context, prompt models.Prompt, opts models.GenerationOptions) (models.Content, error) {
		return models.Content{}, errors.New("failure")
	}
	provider2.GenerateContentFunc = func(ctx context.Context, prompt models.Prompt, opts models.GenerationOptions) (models.Content, error) {
		return models.Content{Title: "Success", Body: "Body", QualityScore: 0.9, ModelUsed: "gpt-4", TokenCount: 100}, nil
	}
	// Setup GetAvailableModels: provider1 fails, provider2 succeeds
	provider1.GetAvailableModelsFunc = func(ctx context.Context) ([]models.ModelInfo, error) {
		return nil, errors.New("provider1 error")
	}
	models2 := []models.ModelInfo{
		{ID: "gpt-4", Name: "GPT-4", QualityScore: 0.95},
	}
	provider2.GetAvailableModelsFunc = func(ctx context.Context) ([]models.ModelInfo, error) {
		return models2, nil
	}

	chain := llm.NewFallbackChain([]llm.LLMProvider{provider1, provider2}, 0.8, mockMetrics)
	// Trip breaker for provider1 via GenerateContent failures
	for i := 0; i < 3; i++ {
		_, _ = chain.GenerateContent(context.Background(), models.Prompt{}, models.GenerationOptions{})
	}
	// Now breaker for provider1 should be open
	// GetAvailableModels should skip provider1 and use provider2
	result, err := chain.GetAvailableModels(context.Background())
	require.NoError(t, err)
	assert.Equal(t, models2, result)
}

func TestFallbackChain_CircuitBreakerOpen_GetModelQualityScore_SkipsProvider(t *testing.T) {
	mockMetrics := &mockMetricsCollector{}
	_ = mockMetrics
	provider1 := &mocks.MockLLMProvider{}
	provider2 := &mocks.MockLLMProvider{}

	// Setup provider1 to fail on GenerateContent (to trip breaker)
	provider1.GenerateContentFunc = func(ctx context.Context, prompt models.Prompt, opts models.GenerationOptions) (models.Content, error) {
		return models.Content{}, errors.New("failure")
	}
	provider2.GenerateContentFunc = func(ctx context.Context, prompt models.Prompt, opts models.GenerationOptions) (models.Content, error) {
		return models.Content{Title: "Success", Body: "Body", QualityScore: 0.9, ModelUsed: "gpt-4", TokenCount: 100}, nil
	}
	// Setup GetModelQualityScore: provider1 fails, provider2 succeeds
	provider1.GetModelQualityScoreFunc = func(ctx context.Context, modelID string) (float64, error) {
		return 0, errors.New("provider1 error")
	}
	provider2.GetModelQualityScoreFunc = func(ctx context.Context, modelID string) (float64, error) {
		return 0.92, nil
	}

	chain := llm.NewFallbackChain([]llm.LLMProvider{provider1, provider2}, 0.8, mockMetrics)
	// Trip breaker for provider1 via GenerateContent failures
	for i := 0; i < 3; i++ {
		_, _ = chain.GenerateContent(context.Background(), models.Prompt{}, models.GenerationOptions{})
	}
	// Now breaker for provider1 should be open
	// GetModelQualityScore should skip provider1 and use provider2
	score, err := chain.GetModelQualityScore(context.Background(), "gpt-4")
	require.NoError(t, err)
	assert.Equal(t, 0.92, score)
}

func TestFallbackChain_MetricsRecordedWhenQualityBelowThreshold(t *testing.T) {
	mockMetrics := &mockMetricsCollector{}
	_ = mockMetrics
	provider1 := &mocks.MockLLMProvider{}

	content := models.Content{
		Title:        "Low Quality",
		Body:         "Body",
		QualityScore: 0.6,
		ModelUsed:    "gpt-3",
		TokenCount:   50,
	}
	provider1.GenerateContentFunc = func(ctx context.Context, prompt models.Prompt, opts models.GenerationOptions) (models.Content, error) {
		return content, nil
	}

	chain := llm.NewFallbackChain([]llm.LLMProvider{provider1}, 0.8, mockMetrics)
	result, err := chain.GenerateContent(context.Background(), models.Prompt{}, models.GenerationOptions{})
	require.NoError(t, err)
	assert.Equal(t, content, result)
	require.Len(t, mockMetrics.recordedLLMQuality, 1)
	assert.Equal(t, "", mockMetrics.recordedLLMQuality[0].repository)
	assert.Equal(t, 0.6, mockMetrics.recordedLLMQuality[0].score)
}
