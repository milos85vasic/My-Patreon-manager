package sync_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/go-github/v69/github"
	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/git"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/content"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/sync"
	"github.com/milos85vasic/My-Patreon-Manager/tests/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenameDetection_Integration(t *testing.T) {
	ctx := context.Background()

	// Two repositories: old repo (to be renamed) and new repo (candidate)
	oldRepo := models.Repository{
		ID:              "old-id",
		Service:         "github",
		Owner:           "owner",
		Name:            "oldname",
		Description:     "Old repository",
		Stars:           10,
		Forks:           2,
		URL:             "https://github.com/owner/oldname",
		HTTPSURL:        "https://github.com/owner/oldname",
		PrimaryLanguage: "Go",
	}
	newRepo := models.Repository{
		ID:              "new-id",
		Service:         "github",
		Owner:           "owner",
		Name:            "newname",
		Description:     "New repository",
		Stars:           20,
		Forks:           5,
		URL:             "https://github.com/owner/newname",
		HTTPSURL:        "https://github.com/owner/newname",
		PrimaryLanguage: "Go",
	}

	// Mock Git provider that returns both repos from ListRepositories
	// but returns 404 for old repo when fetching metadata
	gitMock := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		ListRepositoriesFunc: func(_ context.Context, org string, _ git.ListOptions) ([]models.Repository, error) {
			return []models.Repository{oldRepo, newRepo}, nil
		},
		GetMetadataFunc: func(_ context.Context, repo models.Repository) (models.Repository, error) {
			if repo.ID == oldRepo.ID {
				// Simulate 404 Not Found error
				return models.Repository{}, &github.ErrorResponse{
					Response: &http.Response{StatusCode: 404},
					Message:  "Not Found",
				}
			}
			// For new repo, return metadata (maybe enhanced)
			return repo, nil
		},
		DetectMirrorsFunc: func(_ context.Context, repos []models.Repository) ([]models.MirrorMap, error) {
			return nil, nil
		},
	}

	// Mock Patreon client - may be called for new repo, but we can allow
	patreonMock := &mocks.PatreonClient{
		CampaignIDFunc: func() string { return "campaign-123" },
		CreatePostFunc: func(_ context.Context, _ *models.Post) (*models.Post, error) {
			// Acceptable for new repo
			return &models.Post{}, nil
		},
		ListTiersFunc: func(_ context.Context) ([]models.Tier, error) {
			return []models.Tier{
				{ID: "tier-1", Title: "Bronze", AmountCents: 500},
			}, nil
		},
	}

	// budget := content.NewTokenBudget(100000)
	// gate := content.NewQualityGate(0.75)
	// generator := content.NewGenerator(llmMock, budget, gate, nil, nil, nil)
	var generator *content.Generator = nil

	// Mock database with repository store update expectation
	updateCalled := false
	mockRepoStore := &mocks.MockRepositoryStore{
		UpdateFunc: func(_ context.Context, repo *models.Repository) error {
			updateCalled = true
			// Verify that the old repo's fields are updated to new repo's values
			assert.Equal(t, oldRepo.ID, repo.ID)
			assert.Equal(t, newRepo.Service, repo.Service)
			assert.Equal(t, newRepo.Owner, repo.Owner)
			assert.Equal(t, newRepo.Name, repo.Name)
			assert.Equal(t, newRepo.URL, repo.URL)
			assert.Equal(t, newRepo.HTTPSURL, repo.HTTPSURL)
			return nil
		},
	}
	mockDB := &mocks.MockDatabase{
		RepositoriesFunc: func() database.RepositoryStore { return mockRepoStore },
		AuditEntriesFunc: func() database.AuditEntryStore { return nil }, // skip audit
		AcquireLockFunc:  func(ctx context.Context, lockInfo database.SyncLock) error { return nil },
		ReleaseLockFunc:  func(ctx context.Context) error { return nil },
	}

	orchestrator := sync.NewOrchestrator(mockDB, []git.RepositoryProvider{gitMock}, patreonMock, generator, nil, nil, nil)

	// Run sync (not dry-run)
	result, err := orchestrator.Run(ctx, sync.SyncOptions{})
	require.NoError(t, err)
	// Expect one processed (new repo), one skipped (old repo renamed)
	// Actually old repo is renamed and not processed (skipped), new repo processed
	// Processed count should be 1 (new repo)
	assert.Equal(t, 1, result.Processed)
	assert.Equal(t, 0, result.Failed)
	// Ensure update was called
	assert.True(t, updateCalled, "repository update should have been called")
}
