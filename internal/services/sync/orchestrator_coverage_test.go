package sync

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/git"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/content"
	"github.com/milos85vasic/My-Patreon-Manager/tests/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------- GenerateOnly extended paths ----------

func TestGenerateOnly_MetadataError(t *testing.T) {
	prov := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		ListRepositoriesFunc: func(_ context.Context, _ string, _ git.ListOptions) ([]models.Repository, error) {
			return []models.Repository{{ID: "r1", Service: "github", Owner: "o", Name: "r1"}}, nil
		},
		GetMetadataFunc: func(_ context.Context, _ models.Repository) (models.Repository, error) {
			return models.Repository{}, errors.New("metadata boom")
		},
	}
	gen := content.NewGenerator(nil, nil, nil, nil, nil, nil)
	orc := NewOrchestrator(&mocks.MockDatabase{}, []git.RepositoryProvider{prov}, nil, gen, &mockMetricsCollector{}, slog.Default(), nil)

	res, err := orc.GenerateOnly(context.Background(), SyncOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Failed)
}

func TestGenerateOnly_ArchivedSkipped(t *testing.T) {
	prov := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		ListRepositoriesFunc: func(_ context.Context, _ string, _ git.ListOptions) ([]models.Repository, error) {
			return []models.Repository{{ID: "r1", Service: "github", Owner: "o", Name: "r1"}}, nil
		},
		GetMetadataFunc: func(_ context.Context, _ models.Repository) (models.Repository, error) {
			return models.Repository{IsArchived: true}, nil
		},
	}
	gen := content.NewGenerator(nil, nil, nil, nil, nil, nil)
	orc := NewOrchestrator(&mocks.MockDatabase{}, []git.RepositoryProvider{prov}, nil, gen, &mockMetricsCollector{}, slog.Default(), nil)

	res, err := orc.GenerateOnly(context.Background(), SyncOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Skipped)
}

func TestGenerateOnly_NoMatchingProvider(t *testing.T) {
	prov := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "gitlab" },
		ListRepositoriesFunc: func(_ context.Context, _ string, _ git.ListOptions) ([]models.Repository, error) {
			return []models.Repository{{ID: "r1", Service: "github", Owner: "o", Name: "r1"}}, nil
		},
	}
	gen := content.NewGenerator(nil, nil, nil, nil, nil, nil)
	orc := NewOrchestrator(&mocks.MockDatabase{}, []git.RepositoryProvider{prov}, nil, gen, &mockMetricsCollector{}, slog.Default(), nil)

	res, err := orc.GenerateOnly(context.Background(), SyncOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Skipped)
}

func TestGenerateOnly_GeneratorError(t *testing.T) {
	prov := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		ListRepositoriesFunc: func(_ context.Context, _ string, _ git.ListOptions) ([]models.Repository, error) {
			return []models.Repository{{ID: "r1", Service: "github", Owner: "o", Name: "r1"}}, nil
		},
		GetMetadataFunc: func(_ context.Context, r models.Repository) (models.Repository, error) {
			return r, nil
		},
	}
	llmMock := &mocks.MockLLMProvider{
		GenerateContentFunc: func(_ context.Context, _ models.Prompt, _ models.GenerationOptions) (models.Content, error) {
			return models.Content{}, errors.New("llm error")
		},
	}
	gen := content.NewGenerator(llmMock, nil, content.NewQualityGate(0.0), nil, nil, nil)
	orc := NewOrchestrator(&mocks.MockDatabase{}, []git.RepositoryProvider{prov}, nil, gen, &mockMetricsCollector{}, slog.Default(), nil)

	res, err := orc.GenerateOnly(context.Background(), SyncOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Failed)
}

func TestGenerateOnly_ContextCancelled(t *testing.T) {
	prov := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		ListRepositoriesFunc: func(_ context.Context, _ string, _ git.ListOptions) ([]models.Repository, error) {
			return []models.Repository{
				{ID: "r1", Service: "github", Owner: "o", Name: "r1"},
				{ID: "r2", Service: "github", Owner: "o", Name: "r2"},
			}, nil
		},
	}
	gen := content.NewGenerator(nil, nil, nil, nil, nil, nil)
	orc := NewOrchestrator(&mocks.MockDatabase{}, []git.RepositoryProvider{prov}, nil, gen, &mockMetricsCollector{}, slog.Default(), nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res, err := orc.GenerateOnly(ctx, SyncOptions{})
	require.NoError(t, err)
	assert.Equal(t, 0, res.Processed)
}
