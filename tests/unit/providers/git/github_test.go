package git_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/google/go-github/v69/github"
	"github.com/milos85vasic/My-Patreon-Manager/internal/errors"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/git"
	"github.com/stretchr/testify/assert"
)

func newTestGitHubProvider(t *testing.T, serverURL string) *git.GitHubProvider {
	t.Helper()
	// Create provider with default client (no token)
	provider := git.NewGitHubProvider(nil) // token manager nil, will be set later
	// Set base URL to test server
	err := provider.SetBaseURL(serverURL + "/")
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
	// Set org field if exists
	orgField := val.FieldByName("org")
	if orgField.IsValid() && orgField.CanSet() {
		orgField.SetString("")
	}
	return provider
}

func TestGitHubProvider_Authenticate_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Expect /user/repos?per_page=1
		if r.URL.Path == "/user/repos" && r.URL.Query().Get("per_page") == "1" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode([]*github.Repository{})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := newTestGitHubProvider(t, server.URL)
	err := provider.Authenticate(context.Background(), git.Credentials{PrimaryToken: "test-token"})
	assert.NoError(t, err)
}

func TestGitHubProvider_Authenticate_InvalidToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	provider := newTestGitHubProvider(t, server.URL)
	err := provider.Authenticate(context.Background(), git.Credentials{PrimaryToken: "invalid"})
	assert.Error(t, err)
	assert.True(t, errors.IsInvalidCredentials(err))
}

func TestGitHubProvider_ListRepositories(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/orgs/test-org/repos" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			repo := &github.Repository{
				ID:          github.Int64(1),
				Name:        github.String("repo1"),
				FullName:    github.String("test-org/repo1"),
				Description: github.String("Test repo"),
				HTMLURL:     github.String("https://github.com/test-org/repo1"),
				SSHURL:      github.String("git@github.com:test-org/repo1.git"),
				CloneURL:    github.String("https://github.com/test-org/repo1.git"),
				CreatedAt:   &github.Timestamp{Time: time.Now()},
				UpdatedAt:   &github.Timestamp{Time: time.Now()},
				PushedAt:    &github.Timestamp{Time: time.Now()},
				Size:        github.Int(1024),
				Language:    github.String("Go"),
				Archived:    github.Bool(false),
				Disabled:    github.Bool(false),
				Private:     github.Bool(false),
				Owner:       &github.User{Login: github.String("test-org")},
			}
			json.NewEncoder(w).Encode([]*github.Repository{repo})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := newTestGitHubProvider(t, server.URL)
	repos, err := provider.ListRepositories(context.Background(), "test-org", git.ListOptions{Page: 1, PerPage: 100})
	assert.NoError(t, err)
	assert.Len(t, repos, 1)
	assert.Equal(t, "github", repos[0].Service)
	assert.Equal(t, "test-org", repos[0].Owner)
	assert.Equal(t, "repo1", repos[0].Name)
}

func TestGitHubProvider_GetRepositoryMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/test-org/repo1":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			repo := &github.Repository{
				ID:          github.Int64(1),
				Name:        github.String("repo1"),
				FullName:    github.String("test-org/repo1"),
				Description: github.String("Test repo"),
				HTMLURL:     github.String("https://github.com/test-org/repo1"),
				SSHURL:      github.String("git@github.com:test-org/repo1.git"),
				CloneURL:    github.String("https://github.com/test-org/repo1.git"),
				CreatedAt:   &github.Timestamp{Time: time.Now()},
				UpdatedAt:   &github.Timestamp{Time: time.Now()},
				PushedAt:    &github.Timestamp{Time: time.Now()},
				Size:        github.Int(1024),
				Language:    github.String("Go"),
				Archived:    github.Bool(false),
				Disabled:    github.Bool(false),
				Private:     github.Bool(false),
			}
			json.NewEncoder(w).Encode(repo)
		case "/repos/test-org/repo1/readme":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			readme := &github.RepositoryContent{
				Content: github.String("# README\nTest content"),
			}
			json.NewEncoder(w).Encode(readme)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	provider := newTestGitHubProvider(t, server.URL)
	repo := models.Repository{
		ID:      "test-id",
		Service: "github",
		Owner:   "test-org",
		Name:    "repo1",
	}
	updated, err := provider.GetRepositoryMetadata(context.Background(), repo)
	assert.NoError(t, err)
	assert.Equal(t, "test-id", updated.ID)
	assert.Equal(t, "Test repo", updated.Description)
	assert.Equal(t, "# README\nTest content", updated.READMEContent)
}

func TestGitHubProvider_CheckRepositoryState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/test-org/repo1" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			repo := &github.Repository{
				Archived: github.Bool(false),
				PushedAt: &github.Timestamp{Time: time.Now().Add(-24 * time.Hour)},
			}
			json.NewEncoder(w).Encode(repo)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := newTestGitHubProvider(t, server.URL)
	repo := models.Repository{
		ID:      "test-id",
		Service: "github",
		Owner:   "test-org",
		Name:    "repo1",
	}
	state, err := provider.CheckRepositoryState(context.Background(), repo)
	assert.NoError(t, err)
	assert.Equal(t, "test-id", state.RepositoryID)
	assert.Equal(t, "active", state.Status)
	assert.True(t, state.LastSyncAt.After(time.Now().Add(-48*time.Hour)))
}
