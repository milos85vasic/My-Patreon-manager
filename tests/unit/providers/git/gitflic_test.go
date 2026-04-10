package git_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/errors"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/git"
	"github.com/stretchr/testify/assert"
)

func newTestGitFlicProvider(t *testing.T, serverURL string) *git.GitFlicProvider {
	t.Helper()
	// Create provider with default client (no token)
	provider := git.NewGitFlicProvider(nil) // token manager nil, will be set later
	// Set base URL to test server
	err := provider.SetBaseURL(serverURL + "/api/v1")
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func TestGitFlicProvider_Authenticate_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Expect /api/v1/user to validate token
		if r.URL.Path == "/api/v1/user" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := newTestGitFlicProvider(t, server.URL)
	err := provider.Authenticate(context.Background(), git.Credentials{PrimaryToken: "test-token"})
	assert.NoError(t, err)
}

func TestGitFlicProvider_Authenticate_EmptyToken(t *testing.T) {
	provider := git.NewGitFlicProvider(nil)
	err := provider.Authenticate(context.Background(), git.Credentials{PrimaryToken: ""})
	assert.Error(t, err)
	assert.True(t, errors.IsInvalidCredentials(err))
}

func TestGitFlicProvider_ListRepositories(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/user" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{})
			return
		}
		if r.URL.Path == "/api/v1/orgs/test-org/repos" {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Total-Pages", "1")
			w.WriteHeader(http.StatusOK)
			repos := []map[string]interface{}{
				{
					"id":          1,
					"title":       "Repo One",
					"alias":       "repo1",
					"description": "First test repo",
					"owner":       "Test Owner",
					"ownerAlias":  "test-org",
					"httpUrl":     "https://gitflic.ru/test-org/repo1",
					"sshUrl":      "git@gitflic.ru:test-org/repo1.git",
					"stars":       10,
					"forks":       3,
					"language":    "Go",
					"isPrivate":   false,
				},
			}
			json.NewEncoder(w).Encode(repos)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := newTestGitFlicProvider(t, server.URL)
	err := provider.Authenticate(context.Background(), git.Credentials{PrimaryToken: "test-token"})
	assert.NoError(t, err)
	repos, err := provider.ListRepositories(context.Background(), "test-org", git.ListOptions{Page: 1, PerPage: 100})
	assert.NoError(t, err)
	assert.Len(t, repos, 1)
	assert.Equal(t, "gitflic", repos[0].Service)
	assert.Equal(t, "test-org", repos[0].Owner)
	assert.Equal(t, "repo1", repos[0].Name)
	assert.Equal(t, "First test repo", repos[0].Description)
	assert.Equal(t, "Go", repos[0].PrimaryLanguage)
	assert.Equal(t, 10, repos[0].Stars)
	assert.Equal(t, 3, repos[0].Forks)
	assert.Equal(t, "git@gitflic.ru:test-org/repo1.git", repos[0].URL)
	assert.Equal(t, "https://gitflic.ru/test-org/repo1", repos[0].HTTPSURL)
}

func TestGitFlicProvider_ListRepositories_Pagination(t *testing.T) {
	pageCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/user" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{})
			return
		}
		if r.URL.Path == "/api/v1/orgs/test-org/repos" {
			pageCount++
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Total-Pages", "2")
			w.WriteHeader(http.StatusOK)
			repos := []map[string]interface{}{
				{
					"id":          pageCount,
					"title":       "Repo " + string(rune('A'+pageCount-1)),
					"alias":       "repo" + string(rune('0'+pageCount)),
					"description": "Repo description",
					"owner":       "Test Owner",
					"ownerAlias":  "test-org",
					"httpUrl":     "https://gitflic.ru/test-org/repo" + string(rune('0'+pageCount)),
					"sshUrl":      "git@gitflic.ru:test-org/repo" + string(rune('0'+pageCount)) + ".git",
					"stars":       5,
					"forks":       1,
					"language":    "Go",
					"isPrivate":   false,
				},
			}
			json.NewEncoder(w).Encode(repos)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := newTestGitFlicProvider(t, server.URL)
	err := provider.Authenticate(context.Background(), git.Credentials{PrimaryToken: "test-token"})
	assert.NoError(t, err)
	repos, err := provider.ListRepositories(context.Background(), "test-org", git.ListOptions{Page: 1, PerPage: 1})
	assert.NoError(t, err)
	assert.Len(t, repos, 2)
	assert.Equal(t, "repo1", repos[0].Name)
	assert.Equal(t, "repo2", repos[1].Name)
}

func TestGitFlicProvider_GetRepositoryMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/user" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{})
			return
		}
		if r.URL.Path == "/api/v1/repos/test-owner/repo1" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			repo := map[string]interface{}{
				"title":       "Repo One",
				"alias":       "repo1",
				"description": "Updated description",
				"httpUrl":     "https://gitflic.ru/test-owner/repo1",
				"sshUrl":      "git@gitflic.ru:test-owner/repo1.git",
				"stars":       42,
				"forks":       7,
				"language":    "Python",
			}
			json.NewEncoder(w).Encode(repo)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := newTestGitFlicProvider(t, server.URL)
	err := provider.Authenticate(context.Background(), git.Credentials{PrimaryToken: "test-token"})
	assert.NoError(t, err)
	repo := models.Repository{
		ID:      "test-id",
		Service: "gitflic",
		Owner:   "test-owner",
		Name:    "repo1",
	}
	updatedRepo, err := provider.GetRepositoryMetadata(context.Background(), repo)
	assert.NoError(t, err)
	assert.Equal(t, "test-id", updatedRepo.ID)
	assert.Equal(t, "Updated description", updatedRepo.Description)
	assert.Equal(t, "Python", updatedRepo.PrimaryLanguage)
	assert.Equal(t, 42, updatedRepo.Stars)
	assert.Equal(t, 7, updatedRepo.Forks)
}

func TestGitFlicProvider_CheckRepositoryState(t *testing.T) {
	// GitFlic provider currently returns a default active state
	provider := git.NewGitFlicProvider(nil)
	repo := models.Repository{
		ID:      "test-id",
		Service: "gitflic",
		Owner:   "owner",
		Name:    "repo",
	}
	state, err := provider.CheckRepositoryState(context.Background(), repo)
	assert.NoError(t, err)
	assert.Equal(t, "test-id", state.RepositoryID)
	assert.Equal(t, "active", state.Status)
}
