package sync_test

import (
	"context"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectRename_SameServiceSameOwner(t *testing.T) {
	o := &sync.Orchestrator{}
	repo := models.Repository{
		ID:      "1",
		Service: "github",
		Owner:   "owner",
		Name:    "oldname",
		URL:     "https://github.com/owner/oldname",
	}
	allRepos := []models.Repository{
		repo,
		{
			ID:      "2",
			Service: "github",
			Owner:   "owner",
			Name:    "newname",
			URL:     "https://github.com/owner/newname",
		},
		{
			ID:      "3",
			Service: "gitlab",
			Owner:   "owner",
			Name:    "other",
			URL:     "https://gitlab.com/owner/other",
		},
	}
	candidate, found := o.DetectRename(context.Background(), repo, allRepos)
	assert.True(t, found)
	require.NotNil(t, candidate)
	assert.Equal(t, "newname", candidate.Name)
	assert.Equal(t, "github", candidate.Service)
}

func TestDetectRename_CrossServiceMigration(t *testing.T) {
	o := &sync.Orchestrator{}
	repo := models.Repository{
		ID:      "1",
		Service: "github",
		Owner:   "owner",
		Name:    "repo",
		URL:     "https://github.com/owner/repo",
	}
	allRepos := []models.Repository{
		repo,
		{
			ID:      "2",
			Service: "gitlab",
			Owner:   "owner",
			Name:    "repo",
			URL:     "https://gitlab.com/owner/repo",
		},
	}
	candidate, found := o.DetectRename(context.Background(), repo, allRepos)
	assert.True(t, found)
	require.NotNil(t, candidate)
	assert.Equal(t, "gitlab", candidate.Service)
	assert.Equal(t, "repo", candidate.Name)
}

func TestDetectRename_NotFound(t *testing.T) {
	o := &sync.Orchestrator{}
	repo := models.Repository{
		ID:      "1",
		Service: "github",
		Owner:   "owner",
		Name:    "repo",
		URL:     "https://github.com/owner/repo",
	}
	allRepos := []models.Repository{
		repo,
		{
			ID:      "2",
			Service: "github",
			Owner:   "other",
			Name:    "other",
			URL:     "https://github.com/other/other",
		},
	}
	candidate, found := o.DetectRename(context.Background(), repo, allRepos)
	assert.False(t, found)
	assert.Nil(t, candidate)
}
