package sync

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
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

// ---- PublishOnly ----

// TestOrchestrator_PublishOnly_NoDB exercises the early-return error path
// when the database is unset.
func TestOrchestrator_PublishOnly_NoDB(t *testing.T) {
	orc := NewOrchestrator(nil, nil, &mocks.PatreonClient{}, nil, nil, slog.Default(), nil)
	_, err := orc.PublishOnly(context.Background(), SyncOptions{})
	require.Error(t, err)
}

// TestOrchestrator_PublishOnly_NoPatreon exercises the early-return error
// path when no patreon client is configured.
func TestOrchestrator_PublishOnly_NoPatreon(t *testing.T) {
	orc := NewOrchestrator(&mocks.MockDatabase{}, nil, nil, nil, nil, slog.Default(), nil)
	_, err := orc.PublishOnly(context.Background(), SyncOptions{})
	require.Error(t, err)
}

// fakeRepoStore is an inline test helper exercising the list path without
// pulling in the real SQLite store.
type fakeRepoStore struct {
	repos []*models.Repository
	err   error
}

func (f *fakeRepoStore) Create(context.Context, *models.Repository) error { return nil }
func (f *fakeRepoStore) GetByID(context.Context, string) (*models.Repository, error) {
	return nil, nil
}
func (f *fakeRepoStore) GetByServiceOwnerName(context.Context, string, string, string) (*models.Repository, error) {
	return nil, nil
}
func (f *fakeRepoStore) List(context.Context, database.RepositoryFilter) ([]*models.Repository, error) {
	return f.repos, f.err
}
func (f *fakeRepoStore) Update(context.Context, *models.Repository) error { return nil }
func (f *fakeRepoStore) Delete(context.Context, string) error             { return nil }
func (f *fakeRepoStore) SetRevisionPointers(context.Context, string, string, string) error {
	return nil
}
func (f *fakeRepoStore) SetProcessState(context.Context, string, string) error  { return nil }
func (f *fakeRepoStore) SetLastProcessedAt(context.Context, string, time.Time) error { return nil }

type fakeContentStore struct {
	latest *models.GeneratedContent
	err    error
}

func (f *fakeContentStore) Create(context.Context, *models.GeneratedContent) error { return nil }
func (f *fakeContentStore) GetByID(context.Context, string) (*models.GeneratedContent, error) {
	return nil, nil
}
func (f *fakeContentStore) GetLatestByRepo(context.Context, string) (*models.GeneratedContent, error) {
	return f.latest, f.err
}
func (f *fakeContentStore) GetByQualityRange(context.Context, float64, float64) ([]*models.GeneratedContent, error) {
	return nil, nil
}
func (f *fakeContentStore) ListByRepository(context.Context, string) ([]*models.GeneratedContent, error) {
	return nil, nil
}
func (f *fakeContentStore) Update(context.Context, *models.GeneratedContent) error { return nil }

// TestOrchestrator_PublishOnly_SkipsWhenNoContent verifies that repositories
// without generated content are counted as skipped (not failed).
func TestOrchestrator_PublishOnly_SkipsWhenNoContent(t *testing.T) {
	db := &mocks.MockDatabase{
		RepositoriesFunc: func() database.RepositoryStore {
			return &fakeRepoStore{repos: []*models.Repository{
				{ID: "r1", Service: "github", Owner: "o", Name: "n1"},
				{ID: "r2", Service: "github", Owner: "o", Name: "n2", IsArchived: true},
			}}
		},
		GeneratedContentsFunc: func() database.GeneratedContentStore {
			return &fakeContentStore{latest: nil}
		},
	}
	orc := NewOrchestrator(db, nil, &mocks.PatreonClient{}, nil, nil, slog.Default(), nil)
	res, err := orc.PublishOnly(context.Background(), SyncOptions{})
	require.NoError(t, err)
	assert.Equal(t, 0, res.Processed)
	assert.Equal(t, 2, res.Skipped) // one archived, one without content
}

// TestOrchestrator_PublishOnly_RepoStoreListError exercises the repository
// list error path.
func TestOrchestrator_PublishOnly_RepoStoreListError(t *testing.T) {
	db := &mocks.MockDatabase{
		RepositoriesFunc: func() database.RepositoryStore {
			return &fakeRepoStore{err: errors.New("boom")}
		},
		GeneratedContentsFunc: func() database.GeneratedContentStore {
			return &fakeContentStore{}
		},
	}
	orc := NewOrchestrator(db, nil, &mocks.PatreonClient{}, nil, nil, slog.Default(), nil)
	_, err := orc.PublishOnly(context.Background(), SyncOptions{})
	require.Error(t, err)
}

// TestOrchestrator_PublishOnly_NoRepoStore exercises the nil repo store path.
func TestOrchestrator_PublishOnly_NoRepoStore(t *testing.T) {
	db := &mocks.MockDatabase{
		RepositoriesFunc: func() database.RepositoryStore { return nil },
	}
	orc := NewOrchestrator(db, nil, &mocks.PatreonClient{}, nil, nil, slog.Default(), nil)
	_, err := orc.PublishOnly(context.Background(), SyncOptions{})
	require.Error(t, err)
}

// TestOrchestrator_PublishOnly_NoContentStore exercises the nil content
// store path.
func TestOrchestrator_PublishOnly_NoContentStore(t *testing.T) {
	db := &mocks.MockDatabase{
		RepositoriesFunc:      func() database.RepositoryStore { return &fakeRepoStore{} },
		GeneratedContentsFunc: func() database.GeneratedContentStore { return nil },
	}
	orc := NewOrchestrator(db, nil, &mocks.PatreonClient{}, nil, nil, slog.Default(), nil)
	_, err := orc.PublishOnly(context.Background(), SyncOptions{})
	require.Error(t, err)
}
