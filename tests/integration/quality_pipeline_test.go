package integration

import (
	"context"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/content"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/sync"
	"github.com/milos85vasic/My-Patreon-Manager/tests/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupDB(t *testing.T) *database.SQLiteDB {
	t.Helper()
	db := database.NewSQLiteDB(":memory:")
	require.NoError(t, db.Connect(context.Background(), ""))
	require.NoError(t, db.Migrate(context.Background()))
	t.Cleanup(func() { db.Close() })
	return db
}

func TestContentGenerationPipeline(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()

	llm := &mocks.MockLLMProvider{
		GenerateContentFunc: func(_ context.Context, _ models.Prompt, _ models.GenerationOptions) (models.Content, error) {
			return models.Content{
				Title: "Generated Post", Body: "# My Repo\n\nGreat project!",
				QualityScore: 0.9, ModelUsed: "gpt-4", TokenCount: 250,
			}, nil
		},
	}

	budget := content.NewTokenBudget(100000)
	gate := content.NewQualityGate(0.75)
	gen := content.NewGenerator(llm, budget, gate, db.GeneratedContents(), nil, nil)

	repo := models.Repository{
		ID: "repo-1", Service: "github", Owner: "testowner", Name: "testrepo",
		Description: "A test repository", Stars: 100, Forks: 20,
		HTTPSURL: "https://github.com/testowner/testrepo", PrimaryLanguage: "Go",
	}

	result, err := gen.GenerateForRepository(ctx, repo, nil)
	require.NoError(t, err)
	assert.Equal(t, "Generated Post", result.Title)
	assert.True(t, result.PassedQualityGate)

	stored, err := db.GeneratedContents().GetLatestByRepo(ctx, "repo-1")
	require.NoError(t, err)
	assert.Equal(t, result.ID, stored.ID)
}

func TestContentGeneration_LowQuality(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()

	llm := &mocks.MockLLMProvider{
		GenerateContentFunc: func(_ context.Context, _ models.Prompt, _ models.GenerationOptions) (models.Content, error) {
			return models.Content{
				Title: "Bad", Body: "short", QualityScore: 0.3, ModelUsed: "test", TokenCount: 50,
			}, nil
		},
	}

	gen := content.NewGenerator(llm, content.NewTokenBudget(100000), content.NewQualityGate(0.75), db.GeneratedContents(), nil, nil)
	result, err := gen.GenerateForRepository(ctx, models.Repository{
		ID: "r1", Service: "github", Owner: "o", Name: "n", HTTPSURL: "u",
	}, nil)
	require.NoError(t, err)
	assert.False(t, result.PassedQualityGate)
}

func TestContentGeneration_BudgetExceeded(t *testing.T) {
	db := setupDB(t)
	gen := content.NewGenerator(&mocks.MockLLMProvider{}, content.NewTokenBudget(0), content.NewQualityGate(0.75), db.GeneratedContents(), nil, nil)
	_, err := gen.GenerateForRepository(context.Background(), models.Repository{ID: "r1", Service: "github", Owner: "o", Name: "n", HTTPSURL: "u"}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "token budget")
}

func TestQualityPipeline_ReviewQueue(t *testing.T) {
	budget := content.NewTokenBudget(100000)
	gate := content.NewQualityGate(0.75)
	llm := &mocks.MockLLMProvider{
		GenerateContentFunc: func(_ context.Context, _ models.Prompt, _ models.GenerationOptions) (models.Content, error) {
			return models.Content{Title: "Low", Body: "meh", QualityScore: 0.5, ModelUsed: "test", TokenCount: 100}, nil
		},
	}

	gen := content.NewGenerator(llm, budget, gate, nil, nil, nil)
	result, err := gen.GenerateForRepository(context.Background(), models.Repository{
		ID: "r1", Service: "github", Owner: "o", Name: "n", HTTPSURL: "u",
	}, nil)
	require.NoError(t, err)
	assert.False(t, result.PassedQualityGate)

	rq := content.NewReviewQueue(nil)
	err = rq.AddToReview(context.Background(), result)
	assert.NoError(t, err)
}

func TestDryRunReport(t *testing.T) {
	report := &sync.DryRunReport{
		TotalRepos: 5,
		PlannedActions: []sync.PlannedAction{
			{RepoName: "repo1", ChangeReason: "new commit", ContentType: "promo", Action: "create"},
			{RepoName: "repo2", ChangeReason: "updated", ContentType: "promo", Action: "update"},
		},
		EstimatedAPICalls: 10,
		EstimatedTokens:   5000,
		EstimatedTime:     "30s",
	}

	output := sync.FormatDryRunReport(report, false)
	assert.Contains(t, output, "Total repositories: 5")
	assert.Contains(t, output, "repo1")

	jsonOutput := sync.FormatDryRunReport(report, true)
	assert.Contains(t, jsonOutput, `"total_repos": 5`)
}
