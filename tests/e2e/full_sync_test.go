package e2e

import (
	"context"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/git"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/content"
	"github.com/milos85vasic/My-Patreon-Manager/tests/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFullSyncFlow_MockedProviders(t *testing.T) {
	ctx := context.Background()

	db := database.NewSQLiteDB(":memory:")
	require.NoError(t, db.Connect(ctx, ""))
	require.NoError(t, db.Migrate(ctx))
	defer db.Close()

	ghProvider := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		ListRepositoriesFunc: func(_ context.Context, _ string, _ git.ListOptions) ([]models.Repository, error) {
			return []models.Repository{
				{
					ID: "gh-1", Service: "github", Owner: "testowner", Name: "testrepo",
					URL:         "git@github.com:testowner/testrepo.git",
					HTTPSURL:    "https://github.com/testowner/testrepo",
					Description: "A test repo", Stars: 100, Forks: 20,
					PrimaryLanguage: "Go", Topics: []string{"go", "test"},
				},
			}, nil
		},
	}

	glProvider := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "gitlab" },
		ListRepositoriesFunc: func(_ context.Context, _ string, _ git.ListOptions) ([]models.Repository, error) {
			return []models.Repository{
				{
					ID: "gl-1", Service: "gitlab", Owner: "testowner", Name: "testrepo",
					URL:         "git@gitlab.com:testowner/testrepo.git",
					HTTPSURL:    "https://gitlab.com/testowner/testrepo",
					Description: "A test repo", Stars: 100, Forks: 20,
				},
			}, nil
		},
	}

	ghRepos, err := ghProvider.ListRepositories(ctx, "testowner", git.ListOptions{})
	require.NoError(t, err)
	assert.Len(t, ghRepos, 1)

	glRepos, err := glProvider.ListRepositories(ctx, "testowner", git.ListOptions{})
	require.NoError(t, err)
	assert.Len(t, glRepos, 1)

	for _, repo := range append(ghRepos, glRepos...) {
		err := db.Repositories().Create(ctx, &models.Repository{
			ID: repo.ID, Service: repo.Service, Owner: repo.Owner, Name: repo.Name,
			URL: repo.URL, HTTPSURL: repo.HTTPSURL, Description: repo.Description,
			Stars: repo.Stars, Forks: repo.Forks, PrimaryLanguage: repo.PrimaryLanguage,
		})
		require.NoError(t, err)
	}

	stored, err := db.Repositories().List(ctx, database.RepositoryFilter{})
	require.NoError(t, err)
	assert.Len(t, stored, 2)

	llm := &mocks.MockLLMProvider{
		GenerateContentFunc: func(_ context.Context, _ models.Prompt, _ models.GenerationOptions) (models.Content, error) {
			return models.Content{
				Title: "Check out testrepo!", Body: "# testrepo\n\nA test repo",
				QualityScore: 0.92, ModelUsed: "gpt-4", TokenCount: 300,
			}, nil
		},
	}

	budget := content.NewTokenBudget(100000)
	gate := content.NewQualityGate(0.75)
	gen := content.NewGenerator(llm, budget, gate, db.GeneratedContents(), nil, nil)

	for _, repo := range stored {
		result, err := gen.GenerateForRepository(ctx, *repo, nil)
		require.NoError(t, err)
		assert.True(t, result.PassedQualityGate)
		assert.GreaterOrEqual(t, result.QualityScore, 0.75)

		stored, err := db.GeneratedContents().GetLatestByRepo(ctx, repo.ID)
		require.NoError(t, err)
		assert.Equal(t, result.ID, stored.ID)
	}

	patreon := &mocks.PatreonClient{
		CreatePostFunc: func(_ context.Context, post models.Post) (models.Post, error) {
			post.ID = "patreon-post-1"
			post.PublicationStatus = "published"
			return post, nil
		},
		AssociateTiersFunc: func(_ context.Context, _ string, _ []string) error {
			return nil
		},
	}

	post := models.Post{
		CampaignID: "camp-1",
		Title:      "Check out testrepo!",
		Content:    "# testrepo\n\nA test repo",
		PostType:   "text",
		TierIDs:    []string{"tier-1"},
	}

	created, err := patreon.CreatePost(ctx, post)
	require.NoError(t, err)
	assert.Equal(t, "patreon-post-1", created.ID)
	assert.Equal(t, "published", created.PublicationStatus)
}

func TestNoChangeSync_NoUpdates(t *testing.T) {
	ctx := context.Background()
	db := database.NewSQLiteDB(":memory:")
	require.NoError(t, db.Connect(ctx, ""))
	require.NoError(t, db.Migrate(ctx))
	defer db.Close()

	repo := &models.Repository{
		ID: "r1", Service: "github", Owner: "o", Name: "n",
		URL: "git@github.com:o/n.git", HTTPSURL: "https://github.com/o/n",
		LastCommitSHA: "abc123",
	}
	require.NoError(t, db.Repositories().Create(ctx, repo))

	state := &models.SyncState{
		ID: "s1", RepositoryID: "r1", Status: "synced", LastCommitSHA: "abc123",
	}
	require.NoError(t, db.SyncStates().Create(ctx, state))

	existing, _ := db.SyncStates().GetByRepositoryID(ctx, "r1")
	assert.Equal(t, "abc123", existing.LastCommitSHA)
	assert.Equal(t, "synced", existing.Status)
}
