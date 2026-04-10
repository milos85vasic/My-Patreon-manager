package sync_test

import (
	"context"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/git"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/content"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/sync"
	"github.com/milos85vasic/My-Patreon-Manager/tests/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDryRunOrchestrator_NoPatreonWriteCalls(t *testing.T) {
	ctx := context.Background()

	// Mock Git provider returning two repositories
	gitMock := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		ListRepositoriesFunc: func(_ context.Context, org string, _ git.ListOptions) ([]models.Repository, error) {
			return []models.Repository{
				{
					ID:              "repo-1",
					Service:         "github",
					Owner:           "owner1",
					Name:            "repo1",
					Description:     "First repo",
					Stars:           100,
					Forks:           20,
					HTTPSURL:        "https://github.com/owner1/repo1",
					PrimaryLanguage: "Go",
				},
				{
					ID:              "repo-2",
					Service:         "github",
					Owner:           "owner2",
					Name:            "repo2",
					Description:     "Second repo",
					Stars:           50,
					Forks:           5,
					HTTPSURL:        "https://github.com/owner2/repo2",
					PrimaryLanguage: "Python",
				},
			}, nil
		},
		GetMetadataFunc: func(_ context.Context, repo models.Repository) (models.Repository, error) {
			// Simulate one archived repo
			if repo.ID == "repo-2" {
				repo.IsArchived = true
			}
			return repo, nil
		},
	}

	// Mock LLM provider returning valid content
	llmMock := &mocks.MockLLMProvider{
		GenerateContentFunc: func(_ context.Context, _ models.Prompt, _ models.GenerationOptions) (models.Content, error) {
			return models.Content{
				Title:        "Generated Post",
				Body:         "# My Repo\n\nGreat project!",
				QualityScore: 0.9,
				ModelUsed:    "gpt-4",
				TokenCount:   250,
			}, nil
		},
	}

	// Mock Patreon client - track if any write calls are made
	createPostCalled := false
	patreonMock := &mocks.PatreonClient{
		CampaignIDFunc: func() string { return "campaign-123" },
		CreatePostFunc: func(_ context.Context, _ *models.Post) (*models.Post, error) {
			createPostCalled = true
			return nil, nil
		},
		ListTiersFunc: func(_ context.Context) ([]models.Tier, error) {
			// Return tiers for mapping (should not be called in dry-run)
			return []models.Tier{
				{ID: "tier-1", Title: "Bronze", AmountCents: 500},
				{ID: "tier-2", Title: "Silver", AmountCents: 1000},
				{ID: "tier-3", Title: "Gold", AmountCents: 2000},
			}, nil
		},
	}

	budget := content.NewTokenBudget(100000)
	gate := content.NewQualityGate(0.75)
	generator := content.NewGenerator(llmMock, budget, gate, nil, nil, nil)

	mockDB := &mocks.MockDatabase{}
	mockDB.AcquireLockFunc = func(ctx context.Context, lockInfo database.SyncLock) error { return nil }
	mockDB.ReleaseLockFunc = func(ctx context.Context) error { return nil }

	orchestrator := sync.NewOrchestrator(mockDB, []git.RepositoryProvider{gitMock}, patreonMock, generator, nil, nil, nil)

	// Run sync with dry-run true
	result, err := orchestrator.Run(ctx, sync.SyncOptions{
		DryRun: true,
		Filter: sync.SyncFilter{},
	})
	require.NoError(t, err)
	assert.NotNil(t, result.DryRun)
	assert.Equal(t, 2, result.DryRun.TotalRepos)
	assert.Equal(t, 1, result.Processed) // Only non-archived repo processed
	assert.Equal(t, 0, result.Failed)
	assert.Equal(t, 1, result.Skipped) // Archived repo skipped

	// Verify no Patreon write calls were made
	assert.False(t, createPostCalled, "CreatePost should not be called in dry-run")

	// Verify report contains repo names and would-delete list
	assert.Contains(t, result.DryRun.WouldDelete, "repo2") // archived repo
	assert.Len(t, result.DryRun.PlannedActions, 1)         // only repo1 (non-archived) gets planned action
	if len(result.DryRun.PlannedActions) > 0 {
		action := result.DryRun.PlannedActions[0]
		assert.Equal(t, "repo1", action.RepoName)
		assert.Equal(t, "new", action.ChangeReason)
		assert.Equal(t, "promotional", action.ContentType)
		assert.Equal(t, "create", action.Action)
	}
	// Verify aggregated estimates
	assert.Equal(t, 2, result.DryRun.EstimatedAPICalls)  // 2 API calls per planned action
	assert.Equal(t, 4000, result.DryRun.EstimatedTokens) // 4000 tokens per planned action
	assert.NotEmpty(t, result.DryRun.EstimatedTime)
}

func TestDryRunOrchestrator_ChangeReasons(t *testing.T) {
	ctx := context.Background()

	// Mock Git provider returning a repo with metadata
	gitMock := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		ListRepositoriesFunc: func(_ context.Context, org string, _ git.ListOptions) ([]models.Repository, error) {
			return []models.Repository{
				{
					ID:              "repo-1",
					Service:         "github",
					Owner:           "owner",
					Name:            "repo",
					Description:     "Repo with changes",
					Stars:           150,
					Forks:           30,
					HTTPSURL:        "https://github.com/owner/repo",
					PrimaryLanguage: "Go",
				},
			}, nil
		},
		GetMetadataFunc: func(_ context.Context, repo models.Repository) (models.Repository, error) {
			// Simulate repo not archived
			return repo, nil
		},
	}

	llmMock := &mocks.MockLLMProvider{
		GenerateContentFunc: func(_ context.Context, _ models.Prompt, _ models.GenerationOptions) (models.Content, error) {
			return models.Content{
				Title:        "Generated Post",
				Body:         "# My Repo\n\nGreat project!",
				QualityScore: 0.9,
				ModelUsed:    "gpt-4",
				TokenCount:   250,
			}, nil
		},
	}

	patreonMock := &mocks.PatreonClient{
		CampaignIDFunc: func() string { return "campaign-123" },
		CreatePostFunc: func(_ context.Context, _ *models.Post) (*models.Post, error) {
			t.Error("CreatePost should not be called in dry-run")
			return nil, nil
		},
		ListTiersFunc: func(_ context.Context) ([]models.Tier, error) {
			// This is a read call, allowed in dry-run
			return []models.Tier{
				{ID: "tier-1", Title: "Bronze", AmountCents: 500},
			}, nil
		},
	}

	budget := content.NewTokenBudget(100000)
	gate := content.NewQualityGate(0.75)
	generator := content.NewGenerator(llmMock, budget, gate, nil, nil, nil)

	mockDB := &mocks.MockDatabase{}
	mockDB.AcquireLockFunc = func(ctx context.Context, lockInfo database.SyncLock) error { return nil }
	mockDB.ReleaseLockFunc = func(ctx context.Context) error { return nil }

	orchestrator := sync.NewOrchestrator(mockDB, []git.RepositoryProvider{gitMock}, patreonMock, generator, nil, nil, nil)

	result, err := orchestrator.Run(ctx, sync.SyncOptions{
		DryRun: true,
		Filter: sync.SyncFilter{},
	})
	require.NoError(t, err)
	assert.NotNil(t, result.DryRun)
	assert.Equal(t, 1, result.DryRun.TotalRepos)
	assert.Equal(t, 1, len(result.DryRun.PlannedActions))

	action := result.DryRun.PlannedActions[0]
	assert.Equal(t, "repo", action.RepoName)
	assert.Equal(t, "new", action.ChangeReason)
	assert.Equal(t, "promotional", action.ContentType)
	assert.Equal(t, "create", action.Action)
	// Aggregated estimates
	assert.Equal(t, 2, result.DryRun.EstimatedAPICalls)
	assert.Equal(t, 4000, result.DryRun.EstimatedTokens)
	assert.NotEmpty(t, result.DryRun.EstimatedTime)
}

func TestDryRunOrchestrator_ArchivedRepoWouldDelete(t *testing.T) {
	ctx := context.Background()

	gitMock := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		ListRepositoriesFunc: func(_ context.Context, org string, _ git.ListOptions) ([]models.Repository, error) {
			return []models.Repository{
				{
					ID:              "repo-archived",
					Service:         "github",
					Owner:           "owner",
					Name:            "archived-repo",
					Description:     "Archived repo",
					Stars:           10,
					Forks:           2,
					HTTPSURL:        "https://github.com/owner/archived-repo",
					PrimaryLanguage: "Go",
				},
			}, nil
		},
		GetMetadataFunc: func(_ context.Context, repo models.Repository) (models.Repository, error) {
			repo.IsArchived = true
			return repo, nil
		},
	}

	llmMock := &mocks.MockLLMProvider{
		GenerateContentFunc: func(_ context.Context, _ models.Prompt, _ models.GenerationOptions) (models.Content, error) {
			return models.Content{
				Title:        "Should not be called",
				Body:         "",
				QualityScore: 0.9,
				ModelUsed:    "gpt-4",
				TokenCount:   250,
			}, nil
		},
	}

	patreonMock := &mocks.PatreonClient{
		CampaignIDFunc: func() string { return "campaign-123" },
		CreatePostFunc: func(_ context.Context, _ *models.Post) (*models.Post, error) {
			t.Error("CreatePost should not be called")
			return nil, nil
		},
		ListTiersFunc: func(_ context.Context) ([]models.Tier, error) {
			t.Error("ListTiers should not be called for archived repo")
			return nil, nil
		},
	}

	budget := content.NewTokenBudget(100000)
	gate := content.NewQualityGate(0.75)
	generator := content.NewGenerator(llmMock, budget, gate, nil, nil, nil)

	mockDB := &mocks.MockDatabase{}
	mockDB.AcquireLockFunc = func(ctx context.Context, lockInfo database.SyncLock) error { return nil }
	mockDB.ReleaseLockFunc = func(ctx context.Context) error { return nil }

	orchestrator := sync.NewOrchestrator(mockDB, []git.RepositoryProvider{gitMock}, patreonMock, generator, nil, nil, nil)

	result, err := orchestrator.Run(ctx, sync.SyncOptions{
		DryRun: true,
		Filter: sync.SyncFilter{},
	})
	require.NoError(t, err)
	assert.NotNil(t, result.DryRun)
	assert.Equal(t, 1, result.DryRun.TotalRepos)
	assert.Empty(t, result.DryRun.PlannedActions) // No planned actions for archived repo
	assert.Contains(t, result.DryRun.WouldDelete, "archived-repo")
	assert.Equal(t, 1, len(result.DryRun.WouldDelete))
	// No API calls or tokens estimated
	assert.Equal(t, 0, result.DryRun.EstimatedAPICalls)
	assert.Equal(t, 0, result.DryRun.EstimatedTokens)
}
