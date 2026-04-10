package git_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/errors"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/git"
	"github.com/stretchr/testify/assert"
	"github.com/xanzy/go-gitlab"
)

func newTestGitLabProvider(t *testing.T, serverURL string) *git.GitLabProvider {
	t.Helper()
	// Create provider with default client (no token)
	provider := git.NewGitLabProvider(nil, "") // token manager nil, will be set later
	// Set base URL to test server
	err := provider.SetBaseURL(serverURL + "/api/v4")
	if err != nil {
		t.Fatal(err)
	}
	// Set token manager via reflection (since field is private)
	tm := git.NewTokenManager("test-primary", "test-secondary")
	val := reflect.ValueOf(provider).Elem()
	tmField := val.FieldByName("tm")
	if tmField.IsValid() && tmField.CanSet() {
		tmField.Set(reflect.ValueOf(tm))
	}
	return provider
}

func TestGitLabProvider_Authenticate_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Expect /api/v4/user (or any endpoint) to validate token
		if r.URL.Path == "/api/v4/user" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(&gitlab.User{})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := newTestGitLabProvider(t, server.URL)
	err := provider.Authenticate(context.Background(), git.Credentials{PrimaryToken: "test-token"})
	assert.NoError(t, err)
}

func TestGitLabProvider_Authenticate_EmptyToken(t *testing.T) {
	provider := git.NewGitLabProvider(nil, "")
	err := provider.Authenticate(context.Background(), git.Credentials{PrimaryToken: ""})
	assert.Error(t, err)
	assert.True(t, errors.IsInvalidCredentials(err))
}

func TestGitLabProvider_ListRepositories(t *testing.T) {
	created := time.Now().Add(-30 * 24 * time.Hour)
	updated := time.Now().Add(-1 * time.Hour)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/test-group/projects" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			proj := &gitlab.Project{
				ID:             1,
				Name:           "repo1",
				Path:           "repo1",
				Description:    "Test repo",
				SSHURLToRepo:   "git@gitlab.com:test-group/repo1.git",
				HTTPURLToRepo:  "https://gitlab.com/test-group/repo1",
				CreatedAt:      &created,
				LastActivityAt: &updated,
				Namespace:      &gitlab.ProjectNamespace{Path: "test-group"},
				StarCount:      5,
				ForksCount:     2,
				Archived:       false,
				TagList:        []string{"go", "test"},
			}
			json.NewEncoder(w).Encode([]*gitlab.Project{proj})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := newTestGitLabProvider(t, server.URL)
	repos, err := provider.ListRepositories(context.Background(), "test-group", git.ListOptions{Page: 1, PerPage: 100})
	assert.NoError(t, err)
	assert.Len(t, repos, 1)
	assert.Equal(t, "gitlab", repos[0].Service)
	assert.Equal(t, "test-group", repos[0].Owner)
	assert.Equal(t, "repo1", repos[0].Name)
	assert.Equal(t, "Test repo", repos[0].Description)
	assert.Equal(t, []string{"go", "test"}, repos[0].Topics)
}

func TestGitLabProvider_GetRepositoryMetadata(t *testing.T) {
	created := time.Now().Add(-30 * 24 * time.Hour)
	updated := time.Now().Add(-1 * time.Hour)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Accept both encoded and non-encoded slash
		if strings.HasPrefix(r.URL.Path, "/api/v4/projects/") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			proj := &gitlab.Project{
				ID:             1,
				Name:           "repo1",
				Path:           "repo1",
				Description:    "Test repo",
				SSHURLToRepo:   "git@gitlab.com:test-owner/repo1.git",
				HTTPURLToRepo:  "https://gitlab.com/test-owner/repo1",
				CreatedAt:      &created,
				LastActivityAt: &updated,
				Namespace:      &gitlab.ProjectNamespace{Path: "test-owner"},
				StarCount:      5,
				ForksCount:     2,
				Archived:       false,
				TagList:        []string{"go", "test"},
			}
			json.NewEncoder(w).Encode(proj)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := newTestGitLabProvider(t, server.URL)
	repo := models.Repository{
		ID:      "test-id",
		Service: "gitlab",
		Owner:   "test-owner",
		Name:    "repo1",
	}
	updatedRepo, err := provider.GetRepositoryMetadata(context.Background(), repo)
	assert.NoError(t, err)
	assert.Equal(t, "test-id", updatedRepo.ID)
	assert.Equal(t, "Test repo", updatedRepo.Description)
	assert.Equal(t, []string{"go", "test"}, updatedRepo.Topics)
}

func TestGitLabProvider_CheckRepositoryState(t *testing.T) {
	updated := time.Now().Add(-1 * time.Hour)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v4/projects/") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			proj := &gitlab.Project{
				ID:                  1,
				Name:                "repo1",
				Path:                "repo1",
				Namespace:           &gitlab.ProjectNamespace{Path: "test-owner"},
				LastActivityAt:      &updated,
				MarkedForDeletionAt: nil,
				Archived:            false,
			}
			json.NewEncoder(w).Encode(proj)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := newTestGitLabProvider(t, server.URL)
	repo := models.Repository{
		ID:      "test-id",
		Service: "gitlab",
		Owner:   "test-owner",
		Name:    "repo1",
	}
	state, err := provider.CheckRepositoryState(context.Background(), repo)
	assert.NoError(t, err)
	assert.Equal(t, "test-id", state.RepositoryID)
	assert.Equal(t, "active", state.Status)
	assert.True(t, state.LastSyncAt.After(time.Now().Add(-2*time.Hour)))
}
