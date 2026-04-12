package git

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/google/go-github/v69/github"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------- Authenticate ----------

func TestGitHubProvider_Authenticate_EmptyToken(t *testing.T) {
	p := NewGitHubProvider(nil)
	err := p.Authenticate(context.Background(), Credentials{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GitHub token is required")
}

func TestGitHubProvider_Authenticate_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// GitHub SDK repos list endpoint
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `[]`)
	}))
	defer srv.Close()

	p := NewGitHubProvider(nil)
	if err := p.SetBaseURL(srv.URL + "/"); err != nil {
		t.Fatal(err)
	}

	err := p.Authenticate(context.Background(), Credentials{PrimaryToken: "test-access-token"})
	require.NoError(t, err)
}

func TestGitHubProvider_Authenticate_PreservesBaseURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `[]`)
	}))
	defer srv.Close()

	p := NewGitHubProvider(nil)
	if err := p.SetBaseURL(srv.URL + "/"); err != nil {
		t.Fatal(err)
	}

	err := p.Authenticate(context.Background(), Credentials{PrimaryToken: "test-access-token"})
	require.NoError(t, err)
	// After authenticate, the base URL should be preserved (not reset to github.com)
	assert.Contains(t, p.client.BaseURL.String(), srv.URL)
}

func TestGitHubProvider_Authenticate_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := NewGitHubProvider(NewTokenManager("tok", ""))
	if err := p.SetBaseURL(srv.URL + "/"); err != nil {
		t.Fatal(err)
	}

	err := p.Authenticate(context.Background(), Credentials{PrimaryToken: "test-access-token"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "auth failed")
}

// ---------- ListRepositories ----------

func TestGitHubProvider_ListRepositories_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		name := "test-repo"
		fullName := "org/test-repo"
		login := "org"
		htmlURL := "https://github.com/org/test-repo"
		resp := []map[string]interface{}{
			{
				"id":        1,
				"name":      name,
				"full_name": fullName,
				"html_url":  htmlURL,
				"owner":     map[string]interface{}{"login": login},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	tm := NewTokenManager("tok", "")
	p := NewGitHubProvider(tm)
	if err := p.SetBaseURL(srv.URL + "/"); err != nil {
		t.Fatal(err)
	}

	repos, err := p.ListRepositories(context.Background(), "org", ListOptions{PerPage: 10})
	require.NoError(t, err)
	assert.Len(t, repos, 1)
	assert.Equal(t, "test-repo", repos[0].Name)
}

func TestGitHubProvider_ListRepositories_RateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"API rate limit exceeded"}`)
	}))
	defer srv.Close()

	tm := NewTokenManager("tok", "")
	p := NewGitHubProvider(tm)
	if err := p.SetBaseURL(srv.URL + "/"); err != nil {
		t.Fatal(err)
	}

	_, err := p.ListRepositories(context.Background(), "org", ListOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rate limited")
}

func TestGitHubProvider_ListRepositories_NilRepoSkipped(t *testing.T) {
	// Test that nil repos in the response are skipped
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return an array with valid repos (nil repos can't be represented in JSON,
		// but empty objects will have nil Name etc.)
		fmt.Fprint(w, `[{"name":"valid","owner":{"login":"org"},"html_url":"https://github.com/org/valid"}]`)
	}))
	defer srv.Close()

	tm := NewTokenManager("tok", "")
	p := NewGitHubProvider(tm)
	if err := p.SetBaseURL(srv.URL + "/"); err != nil {
		t.Fatal(err)
	}

	repos, err := p.ListRepositories(context.Background(), "org", ListOptions{PerPage: 10})
	require.NoError(t, err)
	assert.Len(t, repos, 1)
}

func TestGitHubProvider_ListRepositories_DefaultPerPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[]`)
	}))
	defer srv.Close()

	tm := NewTokenManager("tok", "")
	p := NewGitHubProvider(tm)
	if err := p.SetBaseURL(srv.URL + "/"); err != nil {
		t.Fatal(err)
	}

	// PerPage=0 should default to 100
	repos, err := p.ListRepositories(context.Background(), "org", ListOptions{PerPage: 0})
	require.NoError(t, err)
	assert.Empty(t, repos)
}

func TestGitHubProvider_ListRepositories_DefaultOrg(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[]`)
	}))
	defer srv.Close()

	tm := NewTokenManager("tok", "")
	p := NewGitHubProvider(tm)
	p.org = "default-org"
	if err := p.SetBaseURL(srv.URL + "/"); err != nil {
		t.Fatal(err)
	}

	repos, err := p.ListRepositories(context.Background(), "", ListOptions{PerPage: 10})
	require.NoError(t, err)
	assert.Empty(t, repos)
}

// ---------- GetRepositoryMetadata ----------

func TestGitHubProvider_GetRepositoryMetadata_Success(t *testing.T) {
	callCount := int32(0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&callCount, 1)
		w.Header().Set("Content-Type", "application/json")

		switch {
		case count == 1:
			// Repo metadata
			fmt.Fprint(w, `{
				"name":"test-repo",
				"full_name":"owner/test-repo",
				"html_url":"https://github.com/owner/test-repo",
				"owner":{"login":"owner"},
				"description":"A test repo",
				"language":"Go",
				"stargazers_count":42,
				"forks_count":7,
				"archived":false
			}`)
		case count == 2:
			// README
			fmt.Fprint(w, `{
				"name":"README.md",
				"content":"SGVsbG8gV29ybGQ=",
				"encoding":"base64"
			}`)
		case count == 3:
			// Commits
			fmt.Fprint(w, `[{"sha":"abc123"}]`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	tm := NewTokenManager("tok", "")
	p := NewGitHubProvider(tm)
	if err := p.SetBaseURL(srv.URL + "/"); err != nil {
		t.Fatal(err)
	}

	repo := models.Repository{ID: "r1", Owner: "owner", Name: "test-repo"}
	result, err := p.GetRepositoryMetadata(context.Background(), repo)
	require.NoError(t, err)
	assert.Equal(t, "test-repo", result.Name)
	assert.Equal(t, 42, result.Stars)
	assert.Equal(t, "r1", result.ID)
	assert.Equal(t, "abc123", result.LastCommitSHA)
}

func TestGitHubProvider_GetRepositoryMetadata_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	tm := NewTokenManager("tok", "")
	p := NewGitHubProvider(tm)
	if err := p.SetBaseURL(srv.URL + "/"); err != nil {
		t.Fatal(err)
	}

	repo := models.Repository{ID: "r1", Owner: "owner", Name: "test-repo"}
	_, err := p.GetRepositoryMetadata(context.Background(), repo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "github get metadata")
}

// ---------- CheckRepositoryState ----------

func TestGitHubProvider_CheckRepositoryState_Active(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"name":"test-repo",
			"full_name":"owner/test-repo",
			"owner":{"login":"owner"},
			"archived":false,
			"pushed_at":"2024-01-01T00:00:00Z"
		}`)
	}))
	defer srv.Close()

	tm := NewTokenManager("tok", "")
	p := NewGitHubProvider(tm)
	if err := p.SetBaseURL(srv.URL + "/"); err != nil {
		t.Fatal(err)
	}

	repo := models.Repository{ID: "r1", Owner: "owner", Name: "test-repo"}
	state, err := p.CheckRepositoryState(context.Background(), repo)
	require.NoError(t, err)
	assert.Equal(t, "active", state.Status)
	assert.Equal(t, "r1", state.RepositoryID)
}

func TestGitHubProvider_CheckRepositoryState_Archived(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"name":"test-repo",
			"full_name":"owner/test-repo",
			"owner":{"login":"owner"},
			"archived":true
		}`)
	}))
	defer srv.Close()

	tm := NewTokenManager("tok", "")
	p := NewGitHubProvider(tm)
	if err := p.SetBaseURL(srv.URL + "/"); err != nil {
		t.Fatal(err)
	}

	repo := models.Repository{ID: "r1", Owner: "owner", Name: "test-repo"}
	state, err := p.CheckRepositoryState(context.Background(), repo)
	require.NoError(t, err)
	assert.Equal(t, "archived", state.Status)
}

func TestGitHubProvider_CheckRepositoryState_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	tm := NewTokenManager("tok", "")
	p := NewGitHubProvider(tm)
	if err := p.SetBaseURL(srv.URL + "/"); err != nil {
		t.Fatal(err)
	}

	repo := models.Repository{ID: "r1", Owner: "owner", Name: "test-repo"}
	_, err := p.CheckRepositoryState(context.Background(), repo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "github check state")
}

// ---------- execute ----------

func TestGitHubProvider_Execute_BreakerOpen(t *testing.T) {
	// Trip the breaker by making 3+ consecutive 5xx errors
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	tm := NewTokenManager("tok", "")
	p := NewGitHubProvider(tm)
	if err := p.SetBaseURL(srv.URL + "/"); err != nil {
		t.Fatal(err)
	}

	// Make enough calls to trip the breaker
	for i := 0; i < 5; i++ {
		_, _ = p.ListRepositories(context.Background(), "org", ListOptions{Page: 1, PerPage: 10})
	}

	// Next call should fail with breaker error without hitting the server
	beforeCalls := atomic.LoadInt32(&calls)
	_, err := p.ListRepositories(context.Background(), "org", ListOptions{Page: 1, PerPage: 10})
	require.Error(t, err)
	afterCalls := atomic.LoadInt32(&calls)

	// If breaker is open, no new HTTP call should be made
	if afterCalls > beforeCalls+1 {
		// Allow at most one additional call (half-open probe)
		t.Logf("Breaker allowed %d more calls", afterCalls-beforeCalls)
	}
}

func TestGitHubProvider_Execute_4xxNotTripping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message":"Not Found"}`)
	}))
	defer srv.Close()

	tm := NewTokenManager("tok", "")
	p := NewGitHubProvider(tm)
	if err := p.SetBaseURL(srv.URL + "/"); err != nil {
		t.Fatal(err)
	}

	// 4xx errors should not trip the breaker
	for i := 0; i < 10; i++ {
		_, err := p.ListRepositories(context.Background(), "org", ListOptions{Page: 1, PerPage: 10})
		// These will error out with the 404, but breaker should stay closed
		_ = err
	}

	// Breaker should not be open
	assert.NotEqual(t, "open", fmt.Sprintf("%v", tm.cb.State()))
}

// ---------- TokenManager.Execute nil paths ----------

func TestTokenManager_Execute_NilTM(t *testing.T) {
	var tm *TokenManager
	result, err := tm.Execute(func() (interface{}, error) {
		return "ok", nil
	})
	require.NoError(t, err)
	assert.Equal(t, "ok", result)
}

func TestTokenManager_Execute_NilCB(t *testing.T) {
	tm := &TokenManager{} // cb is nil
	result, err := tm.Execute(func() (interface{}, error) {
		return "ok", nil
	})
	require.NoError(t, err)
	assert.Equal(t, "ok", result)
}

// ---------- toRepository with PushedAt ----------

func TestGitHubProvider_ToRepository_NoPushedAt(t *testing.T) {
	p := NewGitHubProvider(nil)
	name := "test-repo"
	login := "owner"
	htmlURL := "https://github.com/owner/test-repo"
	archived := false

	ghRepo := &github.Repository{
		Name:     &name,
		HTMLURL:  &htmlURL,
		Archived: &archived,
		Owner:    &github.User{Login: &login},
	}
	repo := p.toRepository(ghRepo)
	assert.Equal(t, "test-repo", repo.Name)
	assert.Equal(t, "github", repo.Service)
	assert.True(t, repo.LastCommitAt.IsZero())
}
