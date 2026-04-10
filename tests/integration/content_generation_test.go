package integration

import (
	"context"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/llm"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/content"
	"github.com/milos85vasic/My-Patreon-Manager/tests/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ *database.SQLiteDB // ensure import is used

func TestPromptAssembly(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()

	called := false
	llm := &mocks.MockLLMProvider{
		GenerateContentFunc: func(_ context.Context, prompt models.Prompt, _ models.GenerationOptions) (models.Content, error) {
			called = true
			// Verify template name and variables
			assert.Equal(t, "custom", prompt.TemplateName)
			assert.Equal(t, "testrepo", prompt.Variables["REPO_NAME"])
			assert.Equal(t, "testowner", prompt.Variables["REPO_OWNER"])
			assert.Equal(t, "A test repository", prompt.Variables["DESCRIPTION"])
			assert.Equal(t, "100", prompt.Variables["STAR_COUNT"])
			assert.Equal(t, "20", prompt.Variables["FORK_COUNT"])
			assert.Equal(t, "Go", prompt.Variables["LANGUAGE"])
			assert.Equal(t, "github", prompt.Variables["SERVICE"])
			assert.Equal(t, "https://github.com/testowner/testrepo", prompt.Variables["REPO_URL"])
			// Return valid content
			return models.Content{
				Title:        "Generated Post",
				Body:         "# Test\n\nContent",
				QualityScore: 0.9,
				ModelUsed:    "gpt-4",
				TokenCount:   250,
			}, nil
		},
	}

	budget := content.NewTokenBudget(100000)
	gate := content.NewQualityGate(0.75)
	gen := content.NewGenerator(llm, budget, gate, db.GeneratedContents(), nil, nil)

	repo := models.Repository{
		ID:              "repo-1",
		Service:         "github",
		Owner:           "testowner",
		Name:            "testrepo",
		Description:     "A test repository",
		Stars:           100,
		Forks:           20,
		HTTPSURL:        "https://github.com/testowner/testrepo",
		PrimaryLanguage: "Go",
	}

	templates := []models.ContentTemplate{
		{
			Name:     "custom",
			Template: "# {{REPO_NAME}}\n\n{{DESCRIPTION}}\n\n**Language:** {{LANGUAGE}}\n**Stars:** {{STAR_COUNT}} | **Forks:** {{FORK_COUNT}}\n\n[View on {{SERVICE}}]({{REPO_URL}})",
		},
	}

	result, err := gen.GenerateForRepository(ctx, repo, templates, nil)
	require.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, "Generated Post", result.Title)
	assert.True(t, result.PassedQualityGate)
}

func TestQualityScoring(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()

	llm := &mocks.MockLLMProvider{
		GenerateContentFunc: func(_ context.Context, _ models.Prompt, _ models.GenerationOptions) (models.Content, error) {
			return models.Content{
				Title:        "Test",
				Body:         "Body",
				QualityScore: 0.6, // below threshold
				ModelUsed:    "gpt-4",
				TokenCount:   100,
			}, nil
		},
	}

	budget := content.NewTokenBudget(100000)
	gate := content.NewQualityGate(0.75)
	gen := content.NewGenerator(llm, budget, gate, db.GeneratedContents(), nil, nil)

	repo := models.Repository{
		ID:       "repo-1",
		Service:  "github",
		Owner:    "testowner",
		Name:     "testrepo",
		HTTPSURL: "https://github.com/testowner/testrepo",
	}

	result, err := gen.GenerateForRepository(ctx, repo, nil, nil)
	require.NoError(t, err)
	assert.False(t, result.PassedQualityGate)
	assert.Equal(t, 0.6, result.QualityScore)
}

func TestFallbackOnLowScore(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()

	callCount := 0
	lowQualityProvider := &mocks.MockLLMProvider{
		GenerateContentFunc: func(_ context.Context, _ models.Prompt, _ models.GenerationOptions) (models.Content, error) {
			callCount++
			return models.Content{
				Title:        "Low Quality",
				Body:         "Bad",
				QualityScore: 0.5,
				ModelUsed:    "gpt-3.5",
				TokenCount:   50,
			}, nil
		},
	}
	highQualityProvider := &mocks.MockLLMProvider{
		GenerateContentFunc: func(_ context.Context, _ models.Prompt, _ models.GenerationOptions) (models.Content, error) {
			callCount++
			return models.Content{
				Title:        "High Quality",
				Body:         "Good",
				QualityScore: 0.9,
				ModelUsed:    "gpt-4",
				TokenCount:   200,
			}, nil
		},
	}

	fallbackChain := llm.NewFallbackChain([]llm.LLMProvider{lowQualityProvider, highQualityProvider}, 0.75, nil)

	budget := content.NewTokenBudget(100000)
	gate := content.NewQualityGate(0.75)
	gen := content.NewGenerator(fallbackChain, budget, gate, db.GeneratedContents(), nil, nil)

	repo := models.Repository{
		ID:       "repo-1",
		Service:  "github",
		Owner:    "testowner",
		Name:     "testrepo",
		HTTPSURL: "https://github.com/testowner/testrepo",
	}

	result, err := gen.GenerateForRepository(ctx, repo, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, 2, callCount) // both providers called
	assert.True(t, result.PassedQualityGate)
	assert.Equal(t, "High Quality", result.Title)
	assert.Equal(t, 0.9, result.QualityScore)
}

func TestTokenTracking(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()

	llm := &mocks.MockLLMProvider{
		GenerateContentFunc: func(_ context.Context, _ models.Prompt, _ models.GenerationOptions) (models.Content, error) {
			return models.Content{
				Title:        "Test",
				Body:         "Body",
				QualityScore: 0.8,
				ModelUsed:    "gpt-4",
				TokenCount:   500,
			}, nil
		},
	}

	// Budget of 5000 tokens (must be >= MaxTokens 4000)
	budget := content.NewTokenBudget(5000)
	t.Logf("Budget remaining after creation: %d", budget.Remaining())
	gate := content.NewQualityGate(0.75)
	gen := content.NewGenerator(llm, budget, gate, db.GeneratedContents(), nil, nil)

	repo := models.Repository{
		ID:       "repo-1",
		Service:  "github",
		Owner:    "testowner",
		Name:     "testrepo",
		HTTPSURL: "https://github.com/testowner/testrepo",
	}

	// First generation should succeed
	result1, err := gen.GenerateForRepository(ctx, repo, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, 500, result1.TokenCount)
	assert.Equal(t, 4500, budget.Remaining()) // 5000 - 500

	// Second generation also succeeds
	result2, err := gen.GenerateForRepository(ctx, repo, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, 500, result2.TokenCount)
	assert.Equal(t, 4000, budget.Remaining())

	// Third generation should succeed
	result3, err := gen.GenerateForRepository(ctx, repo, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, 500, result3.TokenCount)
	assert.Equal(t, 3500, budget.Remaining())
}
