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

func TestSetProviderOrgs_Nil(t *testing.T) {
	orc := NewOrchestrator(&mocks.MockDatabase{}, nil, nil, nil, nil, slog.Default(), nil)
	orc.SetProviderOrgs(nil)
	assert.Nil(t, orc.ProviderOrgs())
}

func TestSetProviderOrgs_Multiple(t *testing.T) {
	orc := NewOrchestrator(&mocks.MockDatabase{}, nil, nil, nil, nil, slog.Default(), nil)
	orgs := map[string][]string{
		"github": {"org1", "org2"},
		"gitlab": {"group-a"},
	}
	orc.SetProviderOrgs(orgs)
	assert.Equal(t, orgs, orc.ProviderOrgs())
}

func TestDiscoverRepositories_MultiOrg(t *testing.T) {
	calledOrgs := []string{}
	prov := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		ListRepositoriesFunc: func(_ context.Context, org string, _ git.ListOptions) ([]models.Repository, error) {
			calledOrgs = append(calledOrgs, org)
			return []models.Repository{
				{ID: org + "-r1", Service: "github", Owner: org, Name: "repo1"},
			}, nil
		},
	}
	orc := NewOrchestrator(&mocks.MockDatabase{}, []git.RepositoryProvider{prov}, nil, nil, nil, slog.Default(), nil)
	orc.SetProviderOrgs(map[string][]string{
		"github": {"org-a", "org-b"},
	})

	var errs []string
	repos := orc.discoverRepositories(context.Background(), SyncOptions{}, &errs)
	assert.Empty(t, errs)
	assert.Len(t, repos, 2)
	assert.Equal(t, []string{"org-a", "org-b"}, calledOrgs)
}

func TestDiscoverRepositories_MultiOrg_BackwardCompat(t *testing.T) {
	prov := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		ListRepositoriesFunc: func(_ context.Context, org string, _ git.ListOptions) ([]models.Repository, error) {
			assert.Equal(t, "my-org", org)
			return []models.Repository{
				{ID: "r1", Service: "github", Owner: "my-org", Name: "repo1"},
			}, nil
		},
	}
	orc := NewOrchestrator(&mocks.MockDatabase{}, []git.RepositoryProvider{prov}, nil, nil, nil, slog.Default(), nil)

	var errs []string
	repos := orc.discoverRepositories(context.Background(), SyncOptions{Filter: SyncFilter{Org: "my-org"}}, &errs)
	assert.Empty(t, errs)
	assert.Len(t, repos, 1)
}

func TestDiscoverRepositories_MultiOrg_Dedup(t *testing.T) {
	prov := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		ListRepositoriesFunc: func(_ context.Context, org string, _ git.ListOptions) ([]models.Repository, error) {
			return []models.Repository{
				{ID: org + "-r1", Service: "github", Owner: "shared-owner", Name: "shared-repo"},
			}, nil
		},
	}
	orc := NewOrchestrator(&mocks.MockDatabase{}, []git.RepositoryProvider{prov}, nil, nil, nil, slog.Default(), nil)
	orc.SetProviderOrgs(map[string][]string{
		"github": {"org-a", "org-b"},
	})

	var errs []string
	repos := orc.discoverRepositories(context.Background(), SyncOptions{}, &errs)
	assert.Empty(t, errs)
	assert.Len(t, repos, 1)
}

func TestDiscoverRepositories_MultiOrg_PartialError(t *testing.T) {
	callCount := 0
	prov := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		ListRepositoriesFunc: func(_ context.Context, org string, _ git.ListOptions) ([]models.Repository, error) {
			callCount++
			if org == "bad-org" {
				return nil, errors.New("boom")
			}
			return []models.Repository{
				{ID: "r1", Service: "github", Owner: org, Name: "repo1"},
			}, nil
		},
	}
	orc := NewOrchestrator(&mocks.MockDatabase{}, []git.RepositoryProvider{prov}, nil, nil, nil, slog.Default(), nil)
	orc.SetProviderOrgs(map[string][]string{
		"github": {"bad-org", "good-org"},
	})

	var errs []string
	repos := orc.discoverRepositories(context.Background(), SyncOptions{}, &errs)
	assert.Len(t, errs, 1)
	assert.Contains(t, errs[0], "boom")
	assert.Len(t, repos, 1)
	assert.Equal(t, "good-org", repos[0].Owner)
}

func TestDiscoverRepositories_MultiOrg_MultipleProviders(t *testing.T) {
	githubProv := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		ListRepositoriesFunc: func(_ context.Context, org string, _ git.ListOptions) ([]models.Repository, error) {
			return []models.Repository{
				{ID: "gh-" + org, Service: "github", Owner: org, Name: "repo-gh"},
			}, nil
		},
	}
	gitlabProv := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "gitlab" },
		ListRepositoriesFunc: func(_ context.Context, org string, _ git.ListOptions) ([]models.Repository, error) {
			return []models.Repository{
				{ID: "gl-" + org, Service: "gitlab", Owner: org, Name: "repo-gl"},
			}, nil
		},
	}
	orc := NewOrchestrator(
		&mocks.MockDatabase{},
		[]git.RepositoryProvider{githubProv, gitlabProv},
		nil, nil, nil, slog.Default(), nil,
	)
	orc.SetProviderOrgs(map[string][]string{
		"github": {"gh-org1", "gh-org2"},
		"gitlab": {"gl-group"},
	})

	var errs []string
	repos := orc.discoverRepositories(context.Background(), SyncOptions{}, &errs)
	assert.Empty(t, errs)
	assert.Len(t, repos, 3)
}

func TestDiscoverRepositories_MultiOrg_EmptyOrgsFallsBack(t *testing.T) {
	prov := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		ListRepositoriesFunc: func(_ context.Context, org string, _ git.ListOptions) ([]models.Repository, error) {
			assert.Equal(t, "fallback-org", org)
			return []models.Repository{
				{ID: "r1", Service: "github", Owner: "fallback-org", Name: "repo1"},
			}, nil
		},
	}
	orc := NewOrchestrator(&mocks.MockDatabase{}, []git.RepositoryProvider{prov}, nil, nil, nil, slog.Default(), nil)
	orc.SetProviderOrgs(map[string][]string{
		"gitlab": {"some-org"},
	})

	var errs []string
	repos := orc.discoverRepositories(context.Background(), SyncOptions{Filter: SyncFilter{Org: "fallback-org"}}, &errs)
	assert.Empty(t, errs)
	assert.Len(t, repos, 1)
}

func TestDiscoverRepositories_Dedup_SingleOrg(t *testing.T) {
	prov := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		ListRepositoriesFunc: func(_ context.Context, _ string, _ git.ListOptions) ([]models.Repository, error) {
			return []models.Repository{
				{ID: "r1", Service: "github", Owner: "org", Name: "repo"},
				{ID: "r2", Service: "github", Owner: "org", Name: "repo"},
			}, nil
		},
	}
	orc := NewOrchestrator(&mocks.MockDatabase{}, []git.RepositoryProvider{prov}, nil, nil, nil, slog.Default(), nil)

	var errs []string
	repos := orc.discoverRepositories(context.Background(), SyncOptions{Filter: SyncFilter{Org: "org"}}, &errs)
	assert.Empty(t, errs)
	assert.Len(t, repos, 1)
}

func TestScanOnly_MultiOrg(t *testing.T) {
	prov := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		ListRepositoriesFunc: func(_ context.Context, org string, _ git.ListOptions) ([]models.Repository, error) {
			return []models.Repository{
				{ID: org + "-r1", Service: "github", Owner: org, Name: "repo1", URL: "https://github.com/" + org + "/repo1"},
			}, nil
		},
	}
	orc := NewOrchestrator(&mocks.MockDatabase{}, []git.RepositoryProvider{prov}, &mocks.PatreonClient{}, nil, nil, slog.Default(), nil)
	orc.SetProviderOrgs(map[string][]string{
		"github": {"org-a", "org-b"},
	})

	got, err := orc.ScanOnly(context.Background(), SyncOptions{})
	require.NoError(t, err)
	assert.Len(t, got, 2)
}

func TestDiscoverRepositories_MultiOrg_AllErrors(t *testing.T) {
	prov := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		ListRepositoriesFunc: func(_ context.Context, _ string, _ git.ListOptions) ([]models.Repository, error) {
			return nil, errors.New("always fails")
		},
	}
	orc := NewOrchestrator(&mocks.MockDatabase{}, []git.RepositoryProvider{prov}, nil, nil, nil, slog.Default(), nil)
	orc.SetProviderOrgs(map[string][]string{
		"github": {"org-a", "org-b", "org-c"},
	})

	var errs []string
	repos := orc.discoverRepositories(context.Background(), SyncOptions{}, &errs)
	assert.Len(t, errs, 3)
	assert.Empty(t, repos)
}

func TestDiscoverRepositories_MultiOrg_DedupAcrossProviders(t *testing.T) {
	githubProv := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		ListRepositoriesFunc: func(_ context.Context, _ string, _ git.ListOptions) ([]models.Repository, error) {
			return []models.Repository{
				{ID: "gh-r1", Service: "github", Owner: "myorg", Name: "myrepo"},
			}, nil
		},
	}
	gitlabProv := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "gitlab" },
		ListRepositoriesFunc: func(_ context.Context, _ string, _ git.ListOptions) ([]models.Repository, error) {
			return []models.Repository{
				{ID: "gl-r1", Service: "gitlab", Owner: "myorg", Name: "myrepo"},
			}, nil
		},
	}
	orc := NewOrchestrator(
		&mocks.MockDatabase{},
		[]git.RepositoryProvider{githubProv, gitlabProv},
		nil, nil, nil, slog.Default(), nil,
	)

	var errs []string
	repos := orc.discoverRepositories(context.Background(), SyncOptions{Filter: SyncFilter{Org: "myorg"}}, &errs)
	assert.Empty(t, errs)
	assert.Len(t, repos, 1)
	assert.Equal(t, "gh-r1", repos[0].ID)
}

func TestDiscoverRepositories_MultiOrg_NilErrsOut(t *testing.T) {
	prov := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		ListRepositoriesFunc: func(_ context.Context, _ string, _ git.ListOptions) ([]models.Repository, error) {
			return nil, errors.New("fail")
		},
	}
	orc := NewOrchestrator(&mocks.MockDatabase{}, []git.RepositoryProvider{prov}, nil, nil, nil, slog.Default(), nil)
	orc.SetProviderOrgs(map[string][]string{
		"github": {"org-a"},
	})

	repos := orc.discoverRepositories(context.Background(), SyncOptions{}, nil)
	assert.Empty(t, repos)
}
