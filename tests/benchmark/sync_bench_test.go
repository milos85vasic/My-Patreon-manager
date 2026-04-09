package benchmark

import (
	"context"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/content"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/filter"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/sync"
	"github.com/milos85vasic/My-Patreon-Manager/tests/mocks"
)

func BenchmarkSingleRepoSync(b *testing.B) {
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
		gen.GenerateForRepository(context.Background(), repo, nil)
	}
}

func BenchmarkContentGeneration(b *testing.B) {
	_ = &mocks.MockLLMProvider{
		GenerateContentFunc: func(_ context.Context, _ models.Prompt, _ models.GenerationOptions) (models.Content, error) {
			return models.Content{Title: "T", Body: "B", QualityScore: 0.9, TokenCount: 100}, nil
		},
	}
	gate := content.NewQualityGate(0.75)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gate.EvaluateQuality("content", 0.9)
	}
}

func BenchmarkFilterMatching(b *testing.B) {
	repos := make([]models.Repository, 1000)
	for i := range repos {
		repos[i] = models.Repository{
			ID: "r" + string(rune(i)), Owner: "owner", Name: "repo",
			HTTPSURL: "https://github.com/owner/repo",
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sync.ApplyFilter(repos, sync.SyncFilter{Org: "owner"}, nil)
	}
}

func BenchmarkQualityEvaluation(b *testing.B) {
	gate := content.NewQualityGate(0.75)
	c := models.Content{QualityScore: 0.85}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gate.Evaluate(c)
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

func BenchmarkRepoignoreMatch(b *testing.B) {
	r, _ := filter.ParseRepoignoreFile("")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Match("https://github.com/owner/repo")
	}
}
