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

func newTestGitVerseProvider(t *testing.T, serverURL string) *git.GitVerseProvider {
	t.Helper()
	// Create provider with default client (no token)
	provider := git.NewGitVerseProvider(nil) // token manager nil, will be set later
	// Set base URL to test server
	err := provider.SetBaseURL(serverURL + "/api/v1")
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func TestGitVerseProvider_Authenticate_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Capability detection probes /topics and /templates
		if r.URL.Path == "/api/v1/topics" || r.URL.Path == "/api/v1/templates" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := newTestGitVerseProvider(t, server.URL)
	err := provider.Authenticate(context.Background(), git.Credentials{PrimaryToken: "test-token"})
	assert.NoError(t, err)
}

func TestGitVerseProvider_Authenticate_EmptyToken(t *testing.T) {
	provider := git.NewGitVerseProvider(nil)
	err := provider.Authenticate(context.Background(), git.Credentials{PrimaryToken: ""})
	assert.Error(t, err)
	assert.True(t, errors.IsInvalidCredentials(err))
}

func TestGitVerseProvider_Authenticate_CapabilityDetection(t *testing.T) {
	topicsCalled := false
	templatesCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/topics" {
			topicsCalled = true
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode([]string{"go", "docker"})
			return
		}
		if r.URL.Path == "/api/v1/templates" {
			templatesCalled = true
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := newTestGitVerseProvider(t, server.URL)
	err := provider.Authenticate(context.Background(), git.Credentials{PrimaryToken: "test-token"})
	assert.NoError(t, err)
	assert.True(t, topicsCalled, "topics endpoint should be probed")
	assert.True(t, templatesCalled, "templates endpoint should be probed")
	// Capabilities are private; we can't directly assert, but we can test via GetRepositoryMetadata behavior
}

func TestGitVerseProvider_ListRepositories(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Capability probes
		if r.URL.Path == "/api/v1/topics" || r.URL.Path == "/api/v1/templates" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{})
			return
		}
		if r.URL.Path == "/api/v1/orgs/test-org/repos" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			repos := []map[string]interface{}{
				{
					"id":               1,
					"name":             "repo1",
					"full_name":        "test-org/repo1",
					"description":      "First test repo",
					"html_url":         "https://gitverse.ru/test-org/repo1",
					"ssh_url":          "git@gitverse.ru:test-org/repo1.git",
					"stargazers_count": 10,
					"forks_count":      3,
					"language":         "Go",
					"topics":           []string{"go", "test"},
					"archived":         false,
				},
			}
			json.NewEncoder(w).Encode(repos)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := newTestGitVerseProvider(t, server.URL)
	err := provider.Authenticate(context.Background(), git.Credentials{PrimaryToken: "test-token"})
	assert.NoError(t, err)
	repos, err := provider.ListRepositories(context.Background(), "test-org", git.ListOptions{Page: 1, PerPage: 100})
	assert.NoError(t, err)
	assert.Len(t, repos, 1)
	assert.Equal(t, "gitverse", repos[0].Service)
	assert.Equal(t, "test-org", repos[0].Owner)
	assert.Equal(t, "repo1", repos[0].Name)
	assert.Equal(t, "First test repo", repos[0].Description)
	assert.Equal(t, "Go", repos[0].PrimaryLanguage)
	assert.Equal(t, 10, repos[0].Stars)
	assert.Equal(t, 3, repos[0].Forks)
	assert.Equal(t, []string{"go", "test"}, repos[0].Topics)
	assert.Equal(t, "git@gitverse.ru:test-org/repo1.git", repos[0].URL)
	assert.Equal(t, "https://gitverse.ru/test-org/repo1", repos[0].HTTPSURL)
}

func TestGitVerseProvider_ListRepositories_EmptyOrg(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/topics" || r.URL.Path == "/api/v1/templates" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{})
			return
		}
		if r.URL.Path == "/api/v1/user/repos" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			repos := []map[string]interface{}{
				{
					"id":               2,
					"name":             "my-repo",
					"full_name":        "myuser/my-repo",
					"description":      "Personal repo",
					"html_url":         "https://gitverse.ru/myuser/my-repo",
					"ssh_url":          "git@gitverse.ru:myuser/my-repo.git",
					"stargazers_count": 5,
					"forks_count":      1,
					"language":         "Rust",
					"topics":           []string{"rust"},
					"archived":         false,
				},
			}
			json.NewEncoder(w).Encode(repos)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := newTestGitVerseProvider(t, server.URL)
	err := provider.Authenticate(context.Background(), git.Credentials{PrimaryToken: "test-token"})
	assert.NoError(t, err)
	repos, err := provider.ListRepositories(context.Background(), "", git.ListOptions{Page: 1, PerPage: 100})
	assert.NoError(t, err)
	assert.Len(t, repos, 1)
	assert.Equal(t, "gitverse", repos[0].Service)
	assert.Equal(t, "myuser", repos[0].Owner)
	assert.Equal(t, "my-repo", repos[0].Name)
	assert.Equal(t, "Personal repo", repos[0].Description)
	assert.Equal(t, "Rust", repos[0].PrimaryLanguage)
	assert.Equal(t, 5, repos[0].Stars)
	assert.Equal(t, 1, repos[0].Forks)
}

func TestGitVerseProvider_GetRepositoryMetadata_WithTopicsCapability(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Capability probes: topics returns 200, templates 404
		if r.URL.Path == "/api/v1/topics" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode([]string{"sample"})
			return
		}
		if r.URL.Path == "/api/v1/templates" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Path == "/api/v1/repos/test-owner/repo1" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			repo := map[string]interface{}{
				"description":      "Updated description",
				"stargazers_count": 42,
				"forks_count":      7,
				"language":         "Python",
				"topics":           []string{"python", "web"},
				"archived":         false,
			}
			json.NewEncoder(w).Encode(repo)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := newTestGitVerseProvider(t, server.URL)
	err := provider.Authenticate(context.Background(), git.Credentials{PrimaryToken: "test-token"})
	assert.NoError(t, err)
	repo := models.Repository{
		ID:      "test-id",
		Service: "gitverse",
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
	assert.Equal(t, []string{"python", "web"}, updatedRepo.Topics) // topics capability enabled, so topics should be set
}

func TestGitVerseProvider_GetRepositoryMetadata_WithoutTopicsCapability(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Both capability probes return 404
		if r.URL.Path == "/api/v1/topics" || r.URL.Path == "/api/v1/templates" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Path == "/api/v1/repos/test-owner/repo1" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			repo := map[string]interface{}{
				"description":      "Updated description",
				"stargazers_count": 42,
				"forks_count":      7,
				"language":         "Python",
				"topics":           []string{"python", "web"},
				"archived":         false,
			}
			json.NewEncoder(w).Encode(repo)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := newTestGitVerseProvider(t, server.URL)
	err := provider.Authenticate(context.Background(), git.Credentials{PrimaryToken: "test-token"})
	assert.NoError(t, err)
	repo := models.Repository{
		ID:      "test-id",
		Service: "gitverse",
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
	assert.Nil(t, updatedRepo.Topics) // topics capability disabled, so topics should be nil (or empty)
}

func TestGitVerseProvider_GetRepositoryMetadata_NotFoundSetsArchived(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Capability probes
		if r.URL.Path == "/api/v1/topics" || r.URL.Path == "/api/v1/templates" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{})
			return
		}
		if r.URL.Path == "/api/v1/repos/test-owner/repo1" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := newTestGitVerseProvider(t, server.URL)
	err := provider.Authenticate(context.Background(), git.Credentials{PrimaryToken: "test-token"})
	assert.NoError(t, err)
	repo := models.Repository{
		ID:      "test-id",
		Service: "gitverse",
		Owner:   "test-owner",
		Name:    "repo1",
	}
	updatedRepo, err := provider.GetRepositoryMetadata(context.Background(), repo)
	assert.NoError(t, err)
	assert.Equal(t, "test-id", updatedRepo.ID)
	assert.True(t, updatedRepo.IsArchived, "repository should be marked archived on 404")
}

func TestGitVerseProvider_CheckRepositoryState(t *testing.T) {
	// GitVerse provider currently returns a default active state
	provider := git.NewGitVerseProvider(nil)
	repo := models.Repository{
		ID:      "test-id",
		Service: "gitverse",
		Owner:   "owner",
		Name:    "repo",
	}
	state, err := provider.CheckRepositoryState(context.Background(), repo)
	assert.NoError(t, err)
	assert.Equal(t, "test-id", state.RepositoryID)
	assert.Equal(t, "active", state.Status)
}
