package llm

import (
	"context"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

type LLMProvider interface {
	GenerateContent(ctx context.Context, prompt models.Prompt, opts models.GenerationOptions) (models.Content, error)
	GetAvailableModels(ctx context.Context) ([]models.ModelInfo, error)
	GetModelQualityScore(ctx context.Context, modelID string) (float64, error)
	GetTokenUsage(ctx context.Context) (models.UsageStats, error)
}
