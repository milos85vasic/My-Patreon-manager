package sync

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/git"
	"github.com/milos85vasic/My-Patreon-Manager/tests/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- ScanOnly ----

// TestOrchestrator_ScanOnly_Success exercises the happy path: the provider
// returns two repositories and ScanOnly reports them back along with a
// "sync.scan.start" and one "sync.scan" audit entry per repo.
func TestOrchestrator_ScanOnly_Success(t *testing.T) {
	repos := []models.Repository{
		{ID: "r1", Service: "github", Owner: "o", Name: "n1", URL: "https://github.com/o/n1"},
		{ID: "r2", Service: "github", Owner: "o", Name: "n2", URL: "https://github.com/o/n2"},
	}
	prov := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		ListRepositoriesFunc: func(context.Context, string, git.ListOptions) ([]models.Repository, error) {
			return repos, nil
		},
	}
	orc := NewOrchestrator(&mocks.MockDatabase{}, []git.RepositoryProvider{prov}, &mocks.PatreonClient{}, nil, nil, slog.Default(), nil)

	got, err := orc.ScanOnly(context.Background(), SyncOptions{})
	require.NoError(t, err)
	assert.Len(t, got, 2)

	entries, _ := orc.AuditStore().List(context.Background(), 100)
	var sawStart, sawN1, sawN2 bool
	for _, e := range entries {
		switch {
		case e.Action == "sync.scan.start":
			sawStart = true
		case e.Action == "sync.scan" && e.Target == "o/n1":
			sawN1 = true
		case e.Action == "sync.scan" && e.Target == "o/n2":
			sawN2 = true
		}
	}
	assert.True(t, sawStart)
	assert.True(t, sawN1)
	assert.True(t, sawN2)
}

// TestOrchestrator_ScanOnly_ProviderError exercises the provider-failure path:
// one provider returns an error, scan continues (since this is just
// discovery), and an error audit entry is emitted.
func TestOrchestrator_ScanOnly_ProviderError(t *testing.T) {
	prov := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		ListRepositoriesFunc: func(context.Context, string, git.ListOptions) ([]models.Repository, error) {
			return nil, errors.New("listing blew up")
		},
	}
	orc := NewOrchestrator(&mocks.MockDatabase{}, []git.RepositoryProvider{prov}, &mocks.PatreonClient{}, nil, nil, slog.Default(), nil)

	repos, err := orc.ScanOnly(context.Background(), SyncOptions{})
	require.NoError(t, err)
	assert.Empty(t, repos)

	entries, _ := orc.AuditStore().List(context.Background(), 100)
	var sawError bool
	for _, e := range entries {
		if e.Action == "sync.scan" && e.Outcome == "error" {
			sawError = true
			break
		}
	}
	assert.True(t, sawError)
}

// ---- GenerateOnly ----

// TestOrchestrator_GenerateOnly_SkipsWithoutGenerator verifies that when no
// content generator is wired the orchestrator marks each discovered repo as
// skipped instead of crashing.
func TestOrchestrator_GenerateOnly_SkipsWithoutGenerator(t *testing.T) {
	repos := []models.Repository{
		{ID: "r1", Service: "github", Owner: "o", Name: "n1", URL: "https://x"},
	}
	prov := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		ListRepositoriesFunc: func(context.Context, string, git.ListOptions) ([]models.Repository, error) {
			return repos, nil
		},
		GetMetadataFunc: func(_ context.Context, r models.Repository) (models.Repository, error) { return r, nil },
	}
	orc := NewOrchestrator(&mocks.MockDatabase{}, []git.RepositoryProvider{prov}, &mocks.PatreonClient{}, nil, nil, slog.Default(), nil)

	res, err := orc.GenerateOnly(context.Background(), SyncOptions{})
	require.NoError(t, err)
	assert.Equal(t, 0, res.Processed)
	assert.Equal(t, 1, res.Skipped)

	entries, _ := orc.AuditStore().List(context.Background(), 100)
	var sawStart, sawSkipped bool
	for _, e := range entries {
		if e.Action == "sync.generate.start" {
			sawStart = true
		}
		if e.Action == "sync.generate" && e.Outcome == "skipped" {
			sawSkipped = true
		}
	}
	assert.True(t, sawStart)
	assert.True(t, sawSkipped)
}

// TestOrchestrator_GenerateOnly_NoProviders exercises the degenerate case
// where no repositories are discovered — GenerateOnly must return a zero
// result without touching the (nil) generator.
func TestOrchestrator_GenerateOnly_NoProviders(t *testing.T) {
	orc := NewOrchestrator(&mocks.MockDatabase{}, nil, &mocks.PatreonClient{}, nil, nil, slog.Default(), nil)
	res, err := orc.GenerateOnly(context.Background(), SyncOptions{})
	require.NoError(t, err)
	assert.Equal(t, 0, res.Processed)
	assert.Equal(t, 0, res.Skipped)
	assert.Equal(t, 0, res.Failed)
}
