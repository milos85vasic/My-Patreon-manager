package chaos

import (
	"context"
	"errors"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/content"
	"github.com/milos85vasic/My-Patreon-Manager/tests/mocks"
	"github.com/stretchr/testify/assert"
)

func TestContentGeneration_ServiceFailure(t *testing.T) {
	callCount := 0
	llm := &mocks.MockLLMProvider{
		GenerateContentFunc: func(_ context.Context, _ models.Prompt, _ models.GenerationOptions) (models.Content, error) {
			callCount++
			if callCount <= 2 {
				return models.Content{}, errors.New("service unavailable")
			}
			return models.Content{
				Title: "Recovered", Body: "content", QualityScore: 0.85, ModelUsed: "gpt-4", TokenCount: 100,
			}, nil
		},
	}

	budget := content.NewTokenBudget(100000)
	gate := content.NewQualityGate(0.75)
	gen := content.NewGenerator(llm, budget, gate, nil, nil, nil)

	_, err := gen.GenerateForRepository(context.Background(), models.Repository{
		ID: "r1", Service: "github", Owner: "o", Name: "n", HTTPSURL: "u",
	}, nil)

	assert.Error(t, err, "first attempt should fail")
}

func TestContentGeneration_Timeout(t *testing.T) {
	llm := &mocks.MockLLMProvider{
		GenerateContentFunc: func(_ context.Context, _ models.Prompt, _ models.GenerationOptions) (models.Content, error) {
			return models.Content{}, context.DeadlineExceeded
		},
	}

	gen := content.NewGenerator(llm, content.NewTokenBudget(100000), content.NewQualityGate(0.75), nil, nil, nil)

	_, err := gen.GenerateForRepository(context.Background(), models.Repository{
		ID: "r1", Service: "github", Owner: "o", Name: "n", HTTPSURL: "u",
	}, nil)

	assert.Error(t, err)
}

func TestBudgetExhaustion_Graceful(t *testing.T) {
	budget := content.NewTokenBudget(500)

	for i := 0; i < 4; i++ {
		err := budget.CheckBudget(100)
		assert.NoError(t, err)
	}

	err := budget.CheckBudget(200)
	assert.Error(t, err, "should fail after budget exhausted")
}
