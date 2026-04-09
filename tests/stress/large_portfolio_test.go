package stress

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/content"
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

	var wg sync.WaitGroup
	errCount := 0
	var mu sync.Mutex

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := gen.GenerateForRepository(context.Background(), models.Repository{
				ID: "r1", Service: "github", Owner: "o", Name: "n", HTTPSURL: "u",
			}, nil)
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
