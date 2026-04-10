package integration

import (
	"context"
	"testing"
	"time"

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

func TestDryRunReportAccuracy(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()

	// Create three repositories with different change reasons
	// repo1: new (no existing post)
	// repo2: updated (existing post with older hash)
	// repo3: archived (will be marked for deletion)
	repo1 := models.Repository{
		ID:              "repo-1",
		Service:         "github",
		Owner:           "owner1",
		Name:            "repo1",
		Description:     "First repo",
		Stars:           100,
		Forks:           20,
		HTTPSURL:        "https://github.com/owner1/repo1",
		PrimaryLanguage: "Go",
	}
	repo2 := models.Repository{
		ID:              "repo-2",
		Service:         "github",
		Owner:           "owner2",
		Name:            "repo2",
		Description:     "Second repo",
		Stars:           200,
		Forks:           30,
		HTTPSURL:        "https://github.com/owner2/repo2",
		PrimaryLanguage: "Python",
	}
	repo3 := models.Repository{
		ID:              "repo-3",
		Service:         "github",
		Owner:           "owner3",
		Name:            "repo3",
		Description:     "Third repo",
		Stars:           50,
		Forks:           5,
		HTTPSURL:        "https://github.com/owner3/repo3",
		PrimaryLanguage: "JavaScript",
	}

	// Insert repo2 into database with an existing post (simulate previous sync)
	require.NoError(t, db.Repositories().Create(ctx, &repo2))
	postID := "post-2"
	require.NoError(t, db.Posts().Create(ctx, &models.Post{
		ID:                postID,
		CampaignID:        "campaign-123",
		RepositoryID:      repo2.ID,
		Title:             "Old Title",
		Content:           "Old content",
		TierIDs:           []string{"tier-1"},
		ContentHash:       "oldhash",
		PublicationStatus: "published",
		PublishedAt:       time.Time{},
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}))

	// Mock Git provider returning three repos with metadata
	// repo3 will be archived
	gitMock := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		ListRepositoriesFunc: func(_ context.Context, org string, _ git.ListOptions) ([]models.Repository, error) {
			return []models.Repository{repo1, repo2, repo3}, nil
		},
		GetMetadataFunc: func(_ context.Context, repo models.Repository) (models.Repository, error) {
			// Mark repo3 as archived
			if repo.ID == "repo-3" {
				repo.IsArchived = true
			}
			// For repo2, simulate updated metadata (stars changed)
			if repo.ID == "repo-2" {
				repo.Stars = 250 // changed from 200
			}
			return repo, nil
		},
	}

	// Mock LLM provider returning valid content for non-archived repos
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

	// Track Patreon write calls
	var createPostCalls []string
	var updatePostCalls []string
	var deletePostCalls []string
	patreonMock := &mocks.PatreonClient{
		CampaignIDFunc: func() string { return "campaign-123" },
		CreatePostFunc: func(_ context.Context, post *models.Post) (*models.Post, error) {
			createPostCalls = append(createPostCalls, post.RepositoryID)
			return post, nil
		},
		UpdatePostFunc: func(_ context.Context, post *models.Post) (*models.Post, error) {
			updatePostCalls = append(updatePostCalls, post.RepositoryID)
			return post, nil
		},
		DeletePostFunc: func(_ context.Context, postID string) error {
			deletePostCalls = append(deletePostCalls, postID)
			return nil
		},
		ListTiersFunc: func(_ context.Context) ([]models.Tier, error) {
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

	orchestrator := sync.NewOrchestrator(db, []git.RepositoryProvider{gitMock}, patreonMock, generator, nil, nil, nil)

	// Step 1: Dry-run
	resultDry, err := orchestrator.Run(ctx, sync.SyncOptions{
		DryRun: true,
		Filter: sync.SyncFilter{},
	})
	require.NoError(t, err)
	require.NotNil(t, resultDry.DryRun)
	assert.Equal(t, 3, resultDry.DryRun.TotalRepos)
	assert.Equal(t, 2, resultDry.Processed) // repo1 (new) and repo2 (updated)
	assert.Equal(t, 1, resultDry.Skipped)   // repo3 archived

	// Verify planned actions
	require.Len(t, resultDry.DryRun.PlannedActions, 2)
	// Find actions per repo
	var actionRepo1, actionRepo2 *sync.PlannedAction
	for i := range resultDry.DryRun.PlannedActions {
		action := &resultDry.DryRun.PlannedActions[i]
		switch action.RepoName {
		case "repo1":
			actionRepo1 = action
		case "repo2":
			actionRepo2 = action
		}
	}
	require.NotNil(t, actionRepo1, "repo1 should have planned action")
	require.NotNil(t, actionRepo2, "repo2 should have planned action")

	assert.Equal(t, "new", actionRepo1.ChangeReason)
	assert.Equal(t, "create", actionRepo1.Action)
	assert.Equal(t, "updated", actionRepo2.ChangeReason)
	assert.Equal(t, "update", actionRepo2.Action)

	// Verify would-delete contains repo3
	assert.Contains(t, resultDry.DryRun.WouldDelete, "repo3")

	// Verify aggregated estimates
	assert.Equal(t, 4, resultDry.DryRun.EstimatedAPICalls)  // 2 API calls per planned action (create/update + tier mapping)
	assert.Equal(t, 8000, resultDry.DryRun.EstimatedTokens) // 4000 tokens per planned action
	assert.NotEmpty(t, resultDry.DryRun.EstimatedTime)

	// Step 2: Real sync
	// Reset call trackers
	createPostCalls = nil
	updatePostCalls = nil
	deletePostCalls = nil

	resultReal, err := orchestrator.Run(ctx, sync.SyncOptions{
		DryRun: false,
		Filter: sync.SyncFilter{},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, resultReal.Processed)
	assert.Equal(t, 1, resultReal.Skipped)
	assert.Equal(t, 0, resultReal.Failed)

	// Verify actual calls match planned actions
	assert.Equal(t, []string{"repo-1"}, createPostCalls)
	assert.Equal(t, []string{"repo-2"}, updatePostCalls)
	assert.Empty(t, deletePostCalls) // archived repo not deleted (would‑delete only in dry‑run)

	// Verify posts stored in database
	post1, err := db.Posts().GetByRepositoryID(ctx, "repo-1")
	require.NoError(t, err)
	assert.Equal(t, "Generated Post", post1.Title)

	post2, err := db.Posts().GetByRepositoryID(ctx, "repo-2")
	require.NoError(t, err)
	assert.Equal(t, "Generated Post", post2.Title)
	assert.NotEqual(t, "oldhash", post2.ContentHash) // hash should have changed

	// Verify repo3 not processed (no post)
	post3, err := db.Posts().GetByRepositoryID(ctx, "repo-3")
	require.NoError(t, err)
	assert.Nil(t, post3) // not found

	// Verify generated content entries
	gen1, err := db.GeneratedContents().GetLatestByRepo(ctx, "repo-1")
	require.NoError(t, err)
	assert.True(t, gen1.PassedQualityGate)

	gen2, err := db.GeneratedContents().GetLatestByRepo(ctx, "repo-2")
	require.NoError(t, err)
	assert.True(t, gen2.PassedQualityGate)

	// TODO: audit entries not yet implemented
	// audits, err := db.AuditEntries().ListByRepository(ctx, "repo-1")
	// require.NoError(t, err)
	// assert.NotEmpty(t, audits)
	// audits2, err := db.AuditEntries().ListByRepository(ctx, "repo-2")
	// require.NoError(t, err)
	// assert.NotEmpty(t, audits2)
}
