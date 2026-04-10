package integration

import (
	"context"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/git"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/patreon"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/content"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/sync"
	"github.com/milos85vasic/My-Patreon-Manager/tests/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ *database.SQLiteDB // ensure import is used
var _ *patreon.Client    // ensure import is used

func TestSyncPipelineFullFlow(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()

	// Mock Git provider returning one repository
	gitMock := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		ListRepositoriesFunc: func(_ context.Context, org string, _ git.ListOptions) ([]models.Repository, error) {
			return []models.Repository{
				{
					ID:              "repo-1",
					Service:         "github",
					Owner:           "testowner",
					Name:            "testrepo",
					Description:     "A test repository",
					Stars:           100,
					Forks:           20,
					HTTPSURL:        "https://github.com/testowner/testrepo",
					PrimaryLanguage: "Go",
				},
			}, nil
		},
		GetMetadataFunc: func(_ context.Context, repo models.Repository) (models.Repository, error) {
			// Return enhanced repo (same for simplicity)
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

	// Mock Patreon client - track if CreatePost is called
	createPostCalled := false
	var createdPost *models.Post
	patreonMock := &mocks.PatreonClient{
		CampaignIDFunc: func() string { return "campaign-123" },
		CreatePostFunc: func(_ context.Context, post *models.Post) (*models.Post, error) {
			createPostCalled = true
			createdPost = post
			// Return success with same post
			return post, nil
		},
		ListTiersFunc: func(_ context.Context) ([]models.Tier, error) {
			// Return tiers with amounts for tier mapping
			// Linear mapper will map stars+forks = 120 to appropriate tier.
			// We'll set Bronze at 0-200, Silver at 201-500, etc.
			return []models.Tier{
				{ID: "tier-1", Title: "Bronze", AmountCents: 500},
				{ID: "tier-2", Title: "Silver", AmountCents: 1000},
				{ID: "tier-3", Title: "Gold", AmountCents: 2000},
			}, nil
		},
	}

	budget := content.NewTokenBudget(100000)
	gate := content.NewQualityGate(0.75)
	generator := content.NewGenerator(llmMock, budget, gate, db.GeneratedContents(), nil, nil)

	// tierMapper parameter is optional, pass nil to use default linear mapper
	orchestrator := sync.NewOrchestrator(db, []git.RepositoryProvider{gitMock}, patreonMock, generator, nil, nil, nil)

	// Run sync with dry-run false
	result, err := orchestrator.Run(ctx, sync.SyncOptions{
		DryRun: false,
		Filter: sync.SyncFilter{},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Processed)
	assert.Equal(t, 0, result.Failed)
	assert.Equal(t, 0, result.Skipped)

	// Verify generated content was stored
	generated, err := db.GeneratedContents().GetLatestByRepo(ctx, "repo-1")
	require.NoError(t, err)
	assert.Equal(t, "Generated Post", generated.Title)
	assert.True(t, generated.PassedQualityGate)

	// Now verify that Patreon post creation was called
	assert.True(t, createPostCalled, "CreatePost should have been called")
	assert.Equal(t, "Generated Post", createdPost.Title)
	assert.Equal(t, "# My Repo\n\nGreat project!", createdPost.Content)
	assert.Equal(t, "campaign-123", createdPost.CampaignID)
	assert.Equal(t, "repo-1", createdPost.RepositoryID)
	// tier mapping: stars+forks = 120, linear mapper with tiers:
	// Bronze (500) <= 200, Silver (1000) <= 500, Gold (2000) >500
	// Since 120 <= 200, should map to tier-1 (Bronze)
	assert.Contains(t, createdPost.TierIDs, "tier-1", "post should be mapped to Bronze tier")
	assert.Len(t, createdPost.TierIDs, 1)

	// Verify post was stored in database
	post, err := db.Posts().GetByRepositoryID(ctx, "repo-1")
	require.NoError(t, err)
	assert.Equal(t, createdPost.ID, post.ID)
	assert.Equal(t, createdPost.Title, post.Title)
}
