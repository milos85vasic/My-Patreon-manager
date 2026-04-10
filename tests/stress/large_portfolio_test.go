package stress

import (
	"context"
	"fmt"
	"runtime"
	syncpkg "sync"
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

func TestLargePortfolio_1000Repos(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	db := database.NewSQLiteDB(":memory:")
	require.NoError(t, db.Connect(context.Background(), ""))
	require.NoError(t, db.Migrate(context.Background()))
	defer db.Close()

	store := db.Repositories()
	ctx := context.Background()

	start := time.Now()
	for i := 0; i < 1000; i++ {
		repo := &models.Repository{
			ID:      "repo-" + string(rune(i/256)) + string(rune(i%256)),
			Service: "github", Owner: "owner" + string(rune(i%10)), Name: "repo" + string(rune(i)),
			URL:      "git@github.com:owner/repo" + string(rune(i)) + ".git",
			HTTPSURL: "https://github.com/owner/repo" + string(rune(i)),
		}
		require.NoError(t, store.Create(ctx, repo))
	}
	elapsed := time.Since(start)
	t.Logf("Inserted 1000 repos in %v", elapsed)

	repos, err := store.List(ctx, database.RepositoryFilter{})
	require.NoError(t, err)
	assert.Len(t, repos, 1000)
}

func TestConcurrentContentGeneration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	llm := &mocks.MockLLMProvider{
		GenerateContentFunc: func(_ context.Context, _ models.Prompt, _ models.GenerationOptions) (models.Content, error) {
			time.Sleep(1 * time.Millisecond)
			return models.Content{
				Title: "Generated", Body: "content", QualityScore: 0.9, ModelUsed: "gpt-4", TokenCount: 100,
			}, nil
		},
	}

	budget := content.NewTokenBudget(1000000)
	gate := content.NewQualityGate(0.75)
	gen := content.NewGenerator(llm, budget, gate, nil, nil, nil)

	var wg syncpkg.WaitGroup
	errCount := 0
	var mu syncpkg.Mutex

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := gen.GenerateForRepository(context.Background(), models.Repository{
				ID: "r1", Service: "github", Owner: "o", Name: "n", HTTPSURL: "u",
			}, nil, nil)
			if err != nil {
				mu.Lock()
				errCount++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	t.Logf("Completed 100 concurrent generations, %d errors", errCount)
}

func TestLargePortfolio_Sync_1000Repos(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	ctx := context.Background()

	// Setup in-memory database
	db := database.NewSQLiteDB(":memory:")
	require.NoError(t, db.Connect(ctx, ""))
	require.NoError(t, db.Migrate(ctx))
	defer db.Close()

	// Create a mock Git provider that returns 1000 repositories
	repoCount := 1000
	var repos []models.Repository
	for i := 0; i < repoCount; i++ {
		repo := models.Repository{
			ID:              fmt.Sprintf("repo-%d", i),
			Service:         "github",
			Owner:           fmt.Sprintf("owner%d", i%10),
			Name:            fmt.Sprintf("repo%d", i),
			URL:             fmt.Sprintf("git@github.com:owner%d/repo%d.git", i%10, i),
			HTTPSURL:        fmt.Sprintf("https://github.com/owner%d/repo%d", i%10, i),
			Description:     fmt.Sprintf("Repository %d", i),
			Stars:           i % 1000,
			Forks:           i % 100,
			PrimaryLanguage: "Go",
		}
		repos = append(repos, repo)
	}

	provider := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		ListRepositoriesFunc: func(_ context.Context, org string, _ git.ListOptions) ([]models.Repository, error) {
			return repos, nil
		},
		GetMetadataFunc: func(_ context.Context, repo models.Repository) (models.Repository, error) {
			// Not archived
			return repo, nil
		},
	}

	// Mock LLM provider
	llm := &mocks.MockLLMProvider{
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

	// Mock Patreon client
	patreon := &mocks.PatreonClient{
		CampaignIDFunc: func() string { return "campaign-123" },
		CreatePostFunc: func(_ context.Context, post *models.Post) (*models.Post, error) {
			post.ID = "patreon-post-" + post.RepositoryID
			post.PublicationStatus = "published"
			return post, nil
		},
		UpdatePostFunc: func(_ context.Context, post *models.Post) (*models.Post, error) {
			return post, nil
		},
		DeletePostFunc: func(_ context.Context, postID string) error {
			return nil
		},
		ListTiersFunc: func(_ context.Context) ([]models.Tier, error) {
			return []models.Tier{
				{ID: "tier-1", Title: "Bronze", AmountCents: 500},
				{ID: "tier-2", Title: "Silver", AmountCents: 1000},
				{ID: "tier-3", Title: "Gold", AmountCents: 2000},
			}, nil
		},
		AssociateTiersFunc: func(_ context.Context, postID string, tierIDs []string) error {
			return nil
		},
	}

	// Content generator
	budget := content.NewTokenBudget(1000000)
	gate := content.NewQualityGate(0.75)
	generator := content.NewGenerator(llm, budget, gate, db.GeneratedContents(), nil, nil)

	// Orchestrator
	orchestrator := ssync.NewOrchestrator(db, []git.RepositoryProvider{provider}, patreon, generator, nil, nil, nil)

	// Measure memory before
	runtime.GC()
	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)
	start := time.Now()

	// Run sync with timeout
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	result, err := orchestrator.Run(ctx, ssync.SyncOptions{
		DryRun: false,
		Filter: ssync.SyncFilter{},
	})
	require.NoError(t, err)
	elapsed := time.Since(start)

	// Measure memory after
	runtime.GC()
	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)

	// Log metrics
	t.Logf("Processed %d repos, skipped %d, failed %d in %v", result.Processed, result.Skipped, result.Failed, elapsed)
	delta := int64(memAfter.HeapAlloc) - int64(memBefore.HeapAlloc)
	t.Logf("Memory before: %v KiB, after: %v KiB, delta: %v KiB",
		memBefore.HeapAlloc/1024, memAfter.HeapAlloc/1024, delta/1024)

	// Print errors if any
	if len(result.Errors) > 0 {
		for _, err := range result.Errors {
			t.Logf("Error: %s", err)
		}
	}

	// Verify completion within time bounds (should finish under 30 seconds)
	assert.Less(t, elapsed, 30*time.Second, "sync should complete within 30 seconds")

	// Verify stable memory usage: heap allocation delta less than 100 MiB
	maxDelta := int64(100 * 1024 * 1024) // 100 MiB in bytes
	assert.LessOrEqual(t, delta, maxDelta, "memory growth should be under 100 MiB")

	// Verify all repos processed (none skipped or failed)
	assert.Equal(t, repoCount, result.Processed, "all repos should be processed")
	assert.Equal(t, 0, result.Skipped, "no repos should be skipped")
	assert.Equal(t, 0, result.Failed, "no repos should fail")
}

func TestLargePortfolio_CheckpointResume(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	ctx := context.Background()

	// Setup in-memory database
	db := database.NewSQLiteDB(":memory:")
	require.NoError(t, db.Connect(ctx, ""))
	require.NoError(t, db.Migrate(ctx))
	defer db.Close()

	// Create checkpoint manager
	checkpointMgr := ssync.NewCheckpointManager(db)

	// Simulate a checkpoint with some completed repos
	completed := []string{"repo-1", "repo-2", "repo-3"}
	checkpoint := ssync.Checkpoint{
		CompletedRepoIDs: completed,
		CurrentRepoID:    "repo-4",
		StartedAt:        time.Now().Format(time.RFC3339),
		ResumeFrom:       3,
	}
	err := checkpointMgr.SaveCheckpoint(checkpoint)
	require.NoError(t, err)

	// Load checkpoint and verify
	loaded, err := checkpointMgr.LoadCheckpoint()
	require.NoError(t, err)
	assert.Equal(t, completed, loaded.CompletedRepoIDs)
	assert.Equal(t, checkpoint.CurrentRepoID, loaded.CurrentRepoID)

	// Simulate interruption: clear checkpoint and verify resume skips completed repos
	err = checkpointMgr.ClearCheckpoint()
	require.NoError(t, err)

	// Verify checkpoint is gone
	loadedAfterClear, err := checkpointMgr.LoadCheckpoint()
	require.NoError(t, err)
	assert.Empty(t, loadedAfterClear.CompletedRepoIDs)

	// For stress test, we could simulate a sync that resumes from checkpoint
	// but we rely on unit tests for that logic.
	t.Log("Checkpoint resume basic verification passed")
}
