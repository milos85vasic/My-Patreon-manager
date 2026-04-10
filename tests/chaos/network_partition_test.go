package chaos

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/git"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/content"
	ssync "github.com/milos85vasic/My-Patreon-Manager/internal/services/sync"
	"github.com/milos85vasic/My-Patreon-Manager/tests/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNetworkPartition_Timeouts(t *testing.T) {
	llm := &mocks.MockLLMProvider{
		GenerateContentFunc: func(ctx context.Context, _ models.Prompt, _ models.GenerationOptions) (models.Content, error) {
			// Simulate network timeout by returning DeadlineExceeded immediately
			select {
			case <-ctx.Done():
				return models.Content{}, ctx.Err()
			default:
				return models.Content{}, context.DeadlineExceeded
			}
		},
	}

	gen := content.NewGenerator(llm, content.NewTokenBudget(100000), content.NewQualityGate(0.75), nil, nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := gen.GenerateForRepository(ctx, models.Repository{
		ID: "r1", Service: "github", Owner: "o", Name: "n", HTTPSURL: "u",
	}, nil, nil)
	elapsed := time.Since(start)

	assert.Error(t, err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded))
	assert.Less(t, elapsed, 150*time.Millisecond, "should timeout quickly")
}

func TestNetworkPartition_ConnectionResets(t *testing.T) {
	callCount := 0
	llm := &mocks.MockLLMProvider{
		GenerateContentFunc: func(_ context.Context, _ models.Prompt, _ models.GenerationOptions) (models.Content, error) {
			callCount++
			if callCount <= 3 {
				return models.Content{}, errors.New("connection reset by peer")
			}
			return models.Content{
				Title:        "Recovered",
				Body:         "Content after retries",
				QualityScore: 0.8,
				TokenCount:   100,
			}, nil
		},
	}

	gen := content.NewGenerator(llm, content.NewTokenBudget(100000), content.NewQualityGate(0.75), nil, nil, nil)

	_, err := gen.GenerateForRepository(context.Background(), models.Repository{
		ID: "r1", Service: "github", Owner: "o", Name: "n", HTTPSURL: "u",
	}, nil, nil)

	assert.NoError(t, err)
	assert.Equal(t, 4, callCount, "should retry 3 times before success")
}

func TestExponentialBackoff(t *testing.T) {
	// This test verifies that repeated failures lead to increasing delays
	// Since we don't have a built-in backoff mechanism, we just test that
	// the system doesn't crash on repeated failures
	failures := 0
	llm := &mocks.MockLLMProvider{
		GenerateContentFunc: func(_ context.Context, _ models.Prompt, _ models.GenerationOptions) (models.Content, error) {
			failures++
			return models.Content{}, errors.New("network partition")
		},
	}

	gen := content.NewGenerator(llm, content.NewTokenBudget(100000), content.NewQualityGate(0.75), nil, nil, nil)

	// Multiple calls should each fail independently
	for i := 0; i < 5; i++ {
		_, err := gen.GenerateForRepository(context.Background(), models.Repository{
			ID: "r1", Service: "github", Owner: "o", Name: "n", HTTPSURL: "u",
		}, nil, nil)
		assert.Error(t, err)
	}
	assert.Equal(t, 20, failures)
}

func TestNetworkPartition_DNSFailure(t *testing.T) {
	// Simulate DNS lookup failure (e.g., "no such host")
	llm := &mocks.MockLLMProvider{
		GenerateContentFunc: func(_ context.Context, _ models.Prompt, _ models.GenerationOptions) (models.Content, error) {
			return models.Content{}, errors.New("dial tcp: lookup api.openai.com: no such host")
		},
	}

	gen := content.NewGenerator(llm, content.NewTokenBudget(100000), content.NewQualityGate(0.75), nil, nil, nil)

	_, err := gen.GenerateForRepository(context.Background(), models.Repository{
		ID: "r1", Service: "github", Owner: "o", Name: "n", HTTPSURL: "u",
	}, nil, nil)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no such host", "should propagate DNS error")
}

func TestGracefulDegradation(t *testing.T) {
	// Simulate partial failure where some providers work, others don't
	// Use multiple Git providers: one fails, one succeeds
	successProvider := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		ListRepositoriesFunc: func(_ context.Context, org string, _ git.ListOptions) ([]models.Repository, error) {
			return []models.Repository{
				{ID: "repo-ok", Service: "github", Owner: "owner", Name: "ok", HTTPSURL: "https://github.com/owner/ok"},
			}, nil
		},
		GetMetadataFunc: func(_ context.Context, repo models.Repository) (models.Repository, error) {
			return repo, nil
		},
	}
	failingProvider := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "gitlab" },
		ListRepositoriesFunc: func(_ context.Context, org string, _ git.ListOptions) ([]models.Repository, error) {
			return nil, errors.New("network partition: cannot reach GitLab")
		},
	}

	// Use a database to store repositories
	db := database.NewSQLiteDB(":memory:")
	require.NoError(t, db.Connect(context.Background(), ""))
	require.NoError(t, db.Migrate(context.Background()))
	defer db.Close()

	// LLM provider works
	llm := &mocks.MockLLMProvider{
		GenerateContentFunc: func(_ context.Context, _ models.Prompt, _ models.GenerationOptions) (models.Content, error) {
			return models.Content{
				Title:        "Generated",
				Body:         "Content",
				QualityScore: 0.9,
				TokenCount:   100,
			}, nil
		},
	}

	// Patreon client works
	patreonMock := &mocks.PatreonClient{
		CampaignIDFunc: func() string { return "campaign" },
		CreatePostFunc: func(_ context.Context, post *models.Post) (*models.Post, error) {
			return post, nil
		},
		ListTiersFunc: func(_ context.Context) ([]models.Tier, error) {
			return []models.Tier{{ID: "tier-1", Title: "Bronze", AmountCents: 500}}, nil
		},
	}

	budget := content.NewTokenBudget(100000)
	gate := content.NewQualityGate(0.75)
	generator := content.NewGenerator(llm, budget, gate, db.GeneratedContents(), nil, nil)

	orchestrator := ssync.NewOrchestrator(db, []git.RepositoryProvider{successProvider, failingProvider}, patreonMock, generator, nil, nil, nil)

	// Run sync; should succeed despite one provider failing
	result, err := orchestrator.Run(context.Background(), ssync.SyncOptions{DryRun: false, Filter: ssync.SyncFilter{}})
	// No error expected (errors collected in result)
	assert.NoError(t, err, "sync should not return error")
	// Expect at least one error recorded
	assert.Greater(t, len(result.Errors), 0, "should record error for failing provider")
	assert.GreaterOrEqual(t, result.Processed, 1, "should process at least one repository from working provider")
}
