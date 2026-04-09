package sync_test

import (
	"testing"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/sync"
	"github.com/stretchr/testify/assert"
)

func TestApplyFilter_NoFilter(t *testing.T) {
	repos := []models.Repository{
		{ID: "1", Owner: "owner1", Name: "repo1"},
		{ID: "2", Owner: "owner2", Name: "repo2"},
	}
	result := sync.ApplyFilter(repos, sync.SyncFilter{}, nil)
	assert.Len(t, result, 2)
}

func TestApplyFilter_ByOrg(t *testing.T) {
	repos := []models.Repository{
		{ID: "1", Owner: "owner1", Name: "repo1"},
		{ID: "2", Owner: "owner2", Name: "repo2"},
	}
	result := sync.ApplyFilter(repos, sync.SyncFilter{Org: "owner1"}, nil)
	assert.Len(t, result, 1)
	assert.Equal(t, "1", result[0].ID)
}

func TestApplyFilter_ByRepoURL(t *testing.T) {
	repos := []models.Repository{
		{ID: "1", URL: "git@github.com:owner/repo.git", HTTPSURL: "https://github.com/owner/repo"},
		{ID: "2", URL: "git@github.com:other/repo.git", HTTPSURL: "https://github.com/other/repo"},
	}
	result := sync.ApplyFilter(repos, sync.SyncFilter{RepoURL: "https://github.com/owner/repo"}, nil)
	assert.Len(t, result, 1)
	assert.Equal(t, "1", result[0].ID)
}

func TestApplyFilter_ByPattern(t *testing.T) {
	repos := []models.Repository{
		{ID: "1", Name: "my-app"},
		{ID: "2", Name: "my-lib"},
		{ID: "3", Name: "other-project"},
	}
	result := sync.ApplyFilter(repos, sync.SyncFilter{Pattern: "my-*"}, nil)
	assert.Len(t, result, 2)
}

func TestApplyFilter_ByPattern_ExactMatch(t *testing.T) {
	repos := []models.Repository{
		{ID: "1", Name: "repo"},
		{ID: "2", Name: "other"},
	}
	result := sync.ApplyFilter(repos, sync.SyncFilter{Pattern: "repo"}, nil)
	assert.Len(t, result, 1)
}

func TestApplyFilter_ByPattern_StarMatchesAll(t *testing.T) {
	repos := []models.Repository{
		{ID: "1", Name: "anything"},
		{ID: "2", Name: "whatever"},
	}
	result := sync.ApplyFilter(repos, sync.SyncFilter{Pattern: "*"}, nil)
	assert.Len(t, result, 2)
}

func TestApplyFilter_BySince(t *testing.T) {
	repos := []models.Repository{
		{ID: "1", Name: "old", UpdatedAt: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
		{ID: "2", Name: "new", UpdatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
	result := sync.ApplyFilter(repos, sync.SyncFilter{Since: "2024-01-01T00:00:00Z"}, nil)
	assert.Len(t, result, 1)
	assert.Equal(t, "2", result[0].ID)
}

func TestApplyFilter_BySince_InvalidTime(t *testing.T) {
	repos := []models.Repository{
		{ID: "1", Name: "repo1"},
	}
	result := sync.ApplyFilter(repos, sync.SyncFilter{Since: "not-a-time"}, nil)
	assert.Len(t, result, 1)
}

func TestApplyFilter_ChangedOnly(t *testing.T) {
	repos := []models.Repository{
		{ID: "1", Name: "changed"},
		{ID: "2", Name: "unchanged"},
	}
	stateFn := func(repoID string) (*models.SyncState, error) {
		if repoID == "1" {
			return &models.SyncState{LastContentHash: ""}, nil
		}
		return &models.SyncState{LastContentHash: "abc123"}, nil
	}
	result := sync.ApplyFilter(repos, sync.SyncFilter{ChangedOnly: true}, stateFn)
	assert.Len(t, result, 1)
	assert.Equal(t, "1", result[0].ID)
}

func TestApplyFilter_Combined(t *testing.T) {
	repos := []models.Repository{
		{ID: "1", Owner: "myorg", Name: "my-app"},
		{ID: "2", Owner: "myorg", Name: "other"},
		{ID: "3", Owner: "other", Name: "my-app"},
	}
	result := sync.ApplyFilter(repos, sync.SyncFilter{Org: "myorg", Pattern: "my-*"}, nil)
	assert.Len(t, result, 1)
	assert.Equal(t, "1", result[0].ID)
}
