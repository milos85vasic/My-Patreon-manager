package benchmark

import (
	"context"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/git"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/content"
	ssync "github.com/milos85vasic/My-Patreon-Manager/internal/services/sync"
	"github.com/milos85vasic/My-Patreon-Manager/tests/mocks"
)

func BenchmarkFullSync(b *testing.B) {
	// Setup: create in-memory database, mock providers, single repo
	db := database.NewSQLiteDB(":memory:")
	db.Connect(context.Background(), "")
	db.Migrate(context.Background())
	defer db.Close()

	repoStore := db.Repositories()
	ctx := context.Background()
	repo := &models.Repository{
		ID:         "bench-repo",
		Service:    "github",
		Owner:      "owner",
		Name:       "repo",
		URL:        "git@github.com:owner/repo.git",
		HTTPSURL:   "https://github.com/owner/repo",
		IsArchived: false,
	}
	repoStore.Create(ctx, repo)

	// Mock Git provider
	gitMock := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		ListRepositoriesFunc: func(ctx context.Context, org string, opts git.ListOptions) ([]models.Repository, error) {
			return []models.Repository{*repo}, nil
		},
		GetMetadataFunc: func(ctx context.Context, repo models.Repository) (models.Repository, error) {
			return repo, nil
		},
	}
	// Mock LLM provider
	llmMock := &mocks.MockLLMProvider{
		GenerateContentFunc: func(ctx context.Context, prompt models.Prompt, opts models.GenerationOptions) (models.Content, error) {
			return models.Content{
				Title: "Generated", Body: "content", QualityScore: 0.9, ModelUsed: "gpt-4", TokenCount: 100,
			}, nil
		},
	}
	// Mock Patreon client
	patreonMock := &mocks.PatreonClient{
		CampaignIDFunc: func() string { return "campaign-123" },
		CreatePostFunc: func(ctx context.Context, post *models.Post) (*models.Post, error) {
			post.ID = "post-1"
			return post, nil
		},
		UpdatePostFunc: func(ctx context.Context, post *models.Post) (*models.Post, error) {
			return post, nil
		},
		DeletePostFunc: func(ctx context.Context, postID string) error {
			return nil
		},
		ListTiersFunc: func(ctx context.Context) ([]models.Tier, error) {
			return []models.Tier{{ID: "tier-1", Title: "Bronze"}}, nil
		},
		AssociateTiersFunc: func(ctx context.Context, postID string, tierIDs []string) error {
			return nil
		},
	}

	// Content generator
	budget := content.NewTokenBudget(1000000)
	gate := content.NewQualityGate(0.75)
	generator := content.NewGenerator(llmMock, budget, gate, db.GeneratedContents(), nil, nil)

	// Create orchestrator with mocks
	orc := ssync.NewOrchestrator(db, []git.RepositoryProvider{gitMock}, patreonMock, generator, nil, nil, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Run sync with dry-run to avoid side effects
		_, _ = orc.Run(ctx, ssync.SyncOptions{DryRun: true})
	}
}

func BenchmarkSingleRepoSync(b *testing.B) {
	// Benchmark sync for a single repository (real sync)
	// Similar to BenchmarkFullSync but with actual writes disabled via dry-run
	// Use same setup as BenchmarkFullSync
	b.Skip("TODO: implement if needed")
}

func BenchmarkContentGeneration(b *testing.B) {
	llm := &mocks.MockLLMProvider{
		GenerateContentFunc: func(_ context.Context, _ models.Prompt, _ models.GenerationOptions) (models.Content, error) {
			return models.Content{
				Title: "Generated", Body: "content", QualityScore: 0.9, ModelUsed: "gpt-4", TokenCount: 100,
			}, nil
		},
	}

	budget := content.NewTokenBudget(1000000)
	gate := content.NewQualityGate(0.75)
	gen := content.NewGenerator(llm, budget, gate, nil, nil, nil)

	repo := models.Repository{
		ID: "r1", Service: "github", Owner: "owner", Name: "repo",
		Description: "A benchmark repo", Stars: 50, HTTPSURL: "https://github.com/owner/repo",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gen.GenerateForRepository(context.Background(), repo, nil, nil)
	}
}

func BenchmarkQualityGateEvaluation(b *testing.B) {
	gate := content.NewQualityGate(0.75)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gate.EvaluateQuality("content", 0.9)
	}
}

func BenchmarkSQLiteStore(b *testing.B) {
	db := database.NewSQLiteDB(":memory:")
	db.Connect(context.Background(), "")
	db.Migrate(context.Background())
	defer db.Close()

	store := db.Repositories()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		repo := &models.Repository{
			ID:      "bench-" + string(rune(i)),
			Service: "github", Owner: "bench", Name: "repo",
			URL: "git@github.com:bench/repo.git", HTTPSURL: "https://github.com/bench/repo",
		}
		store.Create(ctx, repo)
	}
}
