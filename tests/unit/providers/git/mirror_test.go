package git_test

import (
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/git"
	"github.com/stretchr/testify/assert"
)

func TestMirrorDetector_SameRepoDifferentServices(t *testing.T) {
	detector := git.NewMirrorDetector()

	repos := []models.Repository{
		{ID: "gh-1", Service: "github", Owner: "user", Name: "myrepo", Description: "A great project"},
		{ID: "gl-1", Service: "gitlab", Owner: "user", Name: "myrepo", Description: "A great project"},
	}

	mirrors := detector.DetectMirrors(repos)
	assert.Len(t, mirrors, 2)

	var canonicalCount int
	for _, m := range mirrors {
		if m.IsCanonical {
			canonicalCount++
		}
		assert.GreaterOrEqual(t, m.ConfidenceScore, 0.8)
	}
	assert.Equal(t, 1, canonicalCount)
}

func TestMirrorDetector_PrefersGitHub(t *testing.T) {
	detector := git.NewMirrorDetector()

	repos := []models.Repository{
		{ID: "gl-1", Service: "gitlab", Owner: "user", Name: "repo"},
		{ID: "gh-1", Service: "github", Owner: "user", Name: "repo"},
	}

	mirrors := detector.DetectMirrors(repos)
	for _, m := range mirrors {
		if m.RepositoryID == "gh-1" {
			assert.True(t, m.IsCanonical)
		}
	}
}

func TestMirrorDetector_SameServiceNotMirrored(t *testing.T) {
	detector := git.NewMirrorDetector()

	repos := []models.Repository{
		{ID: "gh-1", Service: "github", Owner: "user", Name: "repo"},
		{ID: "gh-2", Service: "github", Owner: "user", Name: "repo"},
	}

	mirrors := detector.DetectMirrors(repos)
	assert.Empty(t, mirrors)
}

func TestMirrorDetector_DifferentNamesNotMirrored(t *testing.T) {
	detector := git.NewMirrorDetector()

	repos := []models.Repository{
		{ID: "gh-1", Service: "github", Owner: "user", Name: "repo-a"},
		{ID: "gl-1", Service: "gitlab", Owner: "user", Name: "repo-b"},
	}

	mirrors := detector.DetectMirrors(repos)
	assert.Empty(t, mirrors)
}

func TestMirrorDetector_READMEHashMatch(t *testing.T) {
	detector := git.NewMirrorDetector()

	readme := "# My Project\n\nThis is a great project."

	repos := []models.Repository{
		{ID: "gh-1", Service: "github", Owner: "user1", Name: "myrepo", READMEContent: readme},
		{ID: "gl-1", Service: "gitlab", Owner: "user2", Name: "myrepo", READMEContent: readme},
	}

	mirrors := detector.DetectMirrors(repos)
	assert.Len(t, mirrors, 2)
}

func TestMirrorDetector_EmptyInput(t *testing.T) {
	detector := git.NewMirrorDetector()
	mirrors := detector.DetectMirrors(nil)
	assert.Empty(t, mirrors)
}

func TestMirrorDetector_SingleRepo(t *testing.T) {
	detector := git.NewMirrorDetector()
	repos := []models.Repository{
		{ID: "gh-1", Service: "github", Owner: "user", Name: "repo"},
	}
	mirrors := detector.DetectMirrors(repos)
	assert.Empty(t, mirrors)
}

func TestDetectMirrors_Context(t *testing.T) {
	repos := []models.Repository{
		{ID: "gh-1", Service: "github", Owner: "user", Name: "repo", Description: "A project"},
		{ID: "gl-1", Service: "gitlab", Owner: "user", Name: "repo", Description: "A project"},
	}
	mirrors, err := git.DetectMirrors(nil, repos)
	assert.NoError(t, err)
	assert.Len(t, mirrors, 2)
}
