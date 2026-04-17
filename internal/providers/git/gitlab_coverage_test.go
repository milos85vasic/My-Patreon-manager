package git

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------- Authenticate ----------

func TestGitLabProvider_Authenticate_EmptyToken(t *testing.T) {
	p := NewGitLabProvider(nil, "")
	err := p.Authenticate(context.Background(), Credentials{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GitLab token is required")
}

func TestGitLabProvider_Authenticate_Success(t *testing.T) {
	p := NewGitLabProvider(nil, "")
	err := p.Authenticate(context.Background(), Credentials{PrimaryToken: "test-access-token"})
	require.NoError(t, err)
}

func TestGitLabProvider_Authenticate_CustomBaseURL(t *testing.T) {
	p := NewGitLabProvider(nil, "https://custom-gitlab.example.com")
	err := p.Authenticate(context.Background(), Credentials{PrimaryToken: "test-access-token"})
	require.NoError(t, err)
}

func TestGitLabProvider_Authenticate_DefaultBaseURL(t *testing.T) {
	p := NewGitLabProvider(nil, "https://gitlab.com")
	err := p.Authenticate(context.Background(), Credentials{PrimaryToken: "test-access-token"})
	require.NoError(t, err)
}

// ---------- NewGitLabProvider ----------

func TestNewGitLabProvider_WithCustomURL(t *testing.T) {
	p := NewGitLabProvider(nil, "https://custom-gitlab.example.com")
	assert.NotNil(t, p)
	assert.Equal(t, "gitlab", p.Name())
}

func TestNewGitLabProvider_WithDefaultURL(t *testing.T) {
	p := NewGitLabProvider(nil, "https://gitlab.com")
	assert.NotNil(t, p)
}

func TestNewGitLabProvider_EmptyURL(t *testing.T) {
	p := NewGitLabProvider(nil, "")
	assert.NotNil(t, p)
}

// ---------- ListRepositories ----------

func TestGitLabProvider_ListRepositories_RateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	tm := NewTokenManager("tok", "")
	p := NewGitLabProvider(tm, srv.URL)
	if err := p.SetBaseURL(srv.URL); err != nil {
		t.Fatal(err)
	}

	_, err := p.ListRepositories(context.Background(), "group", ListOptions{PerPage: 10})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rate limited")
}

func TestGitLabProvider_ListRepositories_EmptyGroup_UserProjects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v4/projects", r.URL.Path)
		assert.Equal(t, "true", r.URL.Query().Get("owned"))
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{
			"id": 1,
			"path": "my-repo",
			"path_with_namespace": "user/my-repo",
			"ssh_url_to_repo": "git@gitlab.com:user/my-repo.git",
			"http_url_to_repo": "https://gitlab.com/user/my-repo",
			"description": "My project",
			"star_count": 5,
			"forks_count": 1,
			"archived": false,
			"created_at": "2024-01-01T00:00:00Z",
			"last_activity_at": "2024-06-01T00:00:00Z",
			"namespace": {"path": "user"},
			"tag_list": ["go"]
		}]`)
	}))
	defer srv.Close()

	tm := NewTokenManager("tok", "")
	p := NewGitLabProvider(tm, srv.URL)
	if err := p.SetBaseURL(srv.URL); err != nil {
		t.Fatal(err)
	}

	repos, err := p.ListRepositories(context.Background(), "", ListOptions{PerPage: 10})
	require.NoError(t, err)
	require.Len(t, repos, 1)
	assert.Equal(t, "my-repo", repos[0].Name)
	assert.Equal(t, "user", repos[0].Owner)
	assert.Equal(t, "gitlab", repos[0].Service)
}

func TestGitLabProvider_ListRepositories_EmptyGroup_EmptyResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v4/projects", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[]`)
	}))
	defer srv.Close()

	tm := NewTokenManager("tok", "")
	p := NewGitLabProvider(tm, srv.URL)
	if err := p.SetBaseURL(srv.URL); err != nil {
		t.Fatal(err)
	}

	repos, err := p.ListRepositories(context.Background(), "", ListOptions{PerPage: 10})
	require.NoError(t, err)
	assert.Empty(t, repos)
}

func TestGitLabProvider_ListRepositories_GroupPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v4/groups/mygroup/projects", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{
			"id": 2,
			"path": "group-repo",
			"path_with_namespace": "mygroup/group-repo",
			"ssh_url_to_repo": "git@gitlab.com:mygroup/group-repo.git",
			"http_url_to_repo": "https://gitlab.com/mygroup/group-repo",
			"description": "Group project",
			"star_count": 3,
			"forks_count": 0,
			"archived": false,
			"created_at": "2024-01-01T00:00:00Z",
			"last_activity_at": "2024-06-01T00:00:00Z",
			"namespace": {"path": "mygroup"},
			"tag_list": []
		}]`)
	}))
	defer srv.Close()

	tm := NewTokenManager("tok", "")
	p := NewGitLabProvider(tm, srv.URL)
	if err := p.SetBaseURL(srv.URL); err != nil {
		t.Fatal(err)
	}

	repos, err := p.ListRepositories(context.Background(), "mygroup", ListOptions{PerPage: 10})
	require.NoError(t, err)
	require.Len(t, repos, 1)
	assert.Equal(t, "group-repo", repos[0].Name)
	assert.Equal(t, "mygroup", repos[0].Owner)
}

func TestGitLabProvider_ListRepositories_EmptyGroup_RateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	tm := NewTokenManager("tok", "")
	p := NewGitLabProvider(tm, srv.URL)
	if err := p.SetBaseURL(srv.URL); err != nil {
		t.Fatal(err)
	}

	_, err := p.ListRepositories(context.Background(), "", ListOptions{PerPage: 10})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rate limited")
}

func TestGitLabProvider_ListRepositories_DefaultPerPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[]`)
	}))
	defer srv.Close()

	tm := NewTokenManager("tok", "")
	p := NewGitLabProvider(tm, srv.URL)
	if err := p.SetBaseURL(srv.URL); err != nil {
		t.Fatal(err)
	}

	// PerPage=0 should default to 100
	repos, err := p.ListRepositories(context.Background(), "group", ListOptions{PerPage: 0})
	require.NoError(t, err)
	assert.Empty(t, repos)
}

// ---------- GetRepositoryMetadata ----------

func TestGitLabProvider_GetRepositoryMetadata_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	tm := NewTokenManager("tok", "")
	p := NewGitLabProvider(tm, srv.URL)
	if err := p.SetBaseURL(srv.URL); err != nil {
		t.Fatal(err)
	}

	repo := models.Repository{ID: "r1", Owner: "owner", Name: "test-repo"}
	_, err := p.GetRepositoryMetadata(context.Background(), repo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gitlab get metadata")
}

// ---------- CheckRepositoryState ----------

func TestGitLabProvider_CheckRepositoryState_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	tm := NewTokenManager("tok", "")
	p := NewGitLabProvider(tm, srv.URL)
	if err := p.SetBaseURL(srv.URL); err != nil {
		t.Fatal(err)
	}

	repo := models.Repository{ID: "r1", Owner: "owner", Name: "test-repo"}
	_, err := p.CheckRepositoryState(context.Background(), repo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gitlab check state")
}

// ---------- execute ----------

func TestGitLabProvider_Execute_4xxNotTripping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	tm := NewTokenManager("tok", "")
	p := NewGitLabProvider(tm, srv.URL)
	if err := p.SetBaseURL(srv.URL); err != nil {
		t.Fatal(err)
	}

	// 4xx errors should not trip the breaker
	for i := 0; i < 5; i++ {
		_, _ = p.ListRepositories(context.Background(), "group", ListOptions{Page: 1, PerPage: 10})
	}

	// Breaker should not be open
	assert.NotEqual(t, "open", fmt.Sprintf("%v", tm.cb.State()))
}

// ---------- SetBaseURL ----------

func TestGitLabProvider_SetBaseURL_WithToken(t *testing.T) {
	tm := NewTokenManager("tok", "")
	p := NewGitLabProvider(tm, "")

	err := p.SetBaseURL("https://custom-gitlab.example.com")
	require.NoError(t, err)

	// Reset to default
	err = p.SetBaseURL("https://gitlab.com")
	require.NoError(t, err)

	// Empty
	err = p.SetBaseURL("")
	require.NoError(t, err)
}

func TestGitLabProvider_SetBaseURL_NilTokenManager(t *testing.T) {
	p := NewGitLabProvider(nil, "")

	err := p.SetBaseURL("https://custom-gitlab.example.com")
	require.NoError(t, err)
}

// ---------- GetRepositoryMetadata success ----------

func TestGitLabProvider_GetRepositoryMetadata_Success(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		switch {
		case callCount == 1:
			// Project metadata
			fmt.Fprint(w, `{
				"id": 1,
				"path": "test-repo",
				"path_with_namespace": "owner/test-repo",
				"ssh_url_to_repo": "git@gitlab.com:owner/test-repo.git",
				"http_url_to_repo": "https://gitlab.com/owner/test-repo",
				"description": "A test project",
				"star_count": 10,
				"forks_count": 3,
				"archived": false,
				"created_at": "2024-01-01T00:00:00Z",
				"last_activity_at": "2024-06-01T00:00:00Z",
				"namespace": {"path": "owner"},
				"tag_list": ["go", "test"]
			}`)
		case callCount == 2:
			// Commits
			fmt.Fprint(w, `[{"id": "abc123", "message": "initial"}]`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	tm := NewTokenManager("tok", "")
	p := NewGitLabProvider(tm, srv.URL)
	if err := p.SetBaseURL(srv.URL); err != nil {
		t.Fatal(err)
	}

	repo := models.Repository{ID: "r1", Owner: "owner", Name: "test-repo"}
	result, err := p.GetRepositoryMetadata(context.Background(), repo)
	require.NoError(t, err)
	assert.Equal(t, "test-repo", result.Name)
	assert.Equal(t, 10, result.Stars)
	assert.Equal(t, "r1", result.ID)
	assert.Equal(t, "abc123", result.LastCommitSHA)
}

// ---------- CheckRepositoryState success ----------

func TestGitLabProvider_CheckRepositoryState_Active(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"id": 1,
			"path": "test-repo",
			"path_with_namespace": "owner/test-repo",
			"ssh_url_to_repo": "git@gitlab.com:owner/test-repo.git",
			"http_url_to_repo": "https://gitlab.com/owner/test-repo",
			"archived": false,
			"created_at": "2024-01-01T00:00:00Z",
			"last_activity_at": "2024-06-01T00:00:00Z",
			"namespace": {"path": "owner"}
		}`)
	}))
	defer srv.Close()

	tm := NewTokenManager("tok", "")
	p := NewGitLabProvider(tm, srv.URL)
	if err := p.SetBaseURL(srv.URL); err != nil {
		t.Fatal(err)
	}

	repo := models.Repository{ID: "r1", Owner: "owner", Name: "test-repo"}
	state, err := p.CheckRepositoryState(context.Background(), repo)
	require.NoError(t, err)
	assert.Equal(t, "active", state.Status)
	assert.Equal(t, "r1", state.RepositoryID)
}

func TestGitLabProvider_CheckRepositoryState_MarkedForDeletion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"id": 1,
			"path": "test-repo",
			"path_with_namespace": "owner/test-repo",
			"ssh_url_to_repo": "git@gitlab.com:owner/test-repo.git",
			"http_url_to_repo": "https://gitlab.com/owner/test-repo",
			"archived": false,
			"marked_for_deletion_at": "2024-07-01",
			"created_at": "2024-01-01T00:00:00Z",
			"last_activity_at": "2024-06-01T00:00:00Z",
			"namespace": {"path": "owner"}
		}`)
	}))
	defer srv.Close()

	tm := NewTokenManager("tok", "")
	p := NewGitLabProvider(tm, srv.URL)
	if err := p.SetBaseURL(srv.URL); err != nil {
		t.Fatal(err)
	}

	repo := models.Repository{ID: "r1", Owner: "owner", Name: "test-repo"}
	state, err := p.CheckRepositoryState(context.Background(), repo)
	require.NoError(t, err)
	assert.Equal(t, "archived", state.Status)
}
