package mocks

import (
	"context"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

type MockLLMProvider struct {
	GenerateContentFunc      func(ctx context.Context, prompt models.Prompt, opts models.GenerationOptions) (models.Content, error)
	GetAvailableModelsFunc   func(ctx context.Context) ([]models.ModelInfo, error)
	GetModelQualityScoreFunc func(ctx context.Context, modelID string) (float64, error)
	GetTokenUsageFunc        func(ctx context.Context) (models.UsageStats, error)
}

func (m *MockLLMProvider) GenerateContent(ctx context.Context, prompt models.Prompt, opts models.GenerationOptions) (models.Content, error) {
	if m.GenerateContentFunc != nil {
		return m.GenerateContentFunc(ctx, prompt, opts)
	}
	return models.Content{}, nil
}

func (m *MockLLMProvider) GetAvailableModels(ctx context.Context) ([]models.ModelInfo, error) {
	if m.GetAvailableModelsFunc != nil {
		return m.GetAvailableModelsFunc(ctx)
	}
	return nil, nil
}

func (m *MockLLMProvider) GetModelQualityScore(ctx context.Context, modelID string) (float64, error) {
	if m.GetModelQualityScoreFunc != nil {
		return m.GetModelQualityScoreFunc(ctx, modelID)
	}
	return 0.0, nil
}

func (m *MockLLMProvider) GetTokenUsage(ctx context.Context) (models.UsageStats, error) {
	if m.GetTokenUsageFunc != nil {
		return m.GetTokenUsageFunc(ctx)
	}
	return models.UsageStats{}, nil
}
