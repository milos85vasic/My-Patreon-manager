package git

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

// ---- GitFlic ----

func TestGitFlicProvider_Authenticate_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"login":"testuser"}`))
	}))
	defer server.Close()

	p := NewGitFlicProvider(nil)
	p.SetBaseURL(server.URL)

	err := p.Authenticate(context.Background(), Credentials{PrimaryToken: "test-token"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestGitFlicProvider_Authenticate_EmptyToken(t *testing.T) {
	p := NewGitFlicProvider(nil)
	err := p.Authenticate(context.Background(), Credentials{})
	if err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestGitFlicProvider_Authenticate_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	p := NewGitFlicProvider(NewTokenManager("tok", ""))
	p.SetBaseURL(server.URL)

	err := p.Authenticate(context.Background(), Credentials{PrimaryToken: "test-token"})
	if err == nil {
		t.Fatal("expected error for server error")
	}
}

func TestGitFlicProvider_Authenticate_BadStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	p := NewGitFlicProvider(NewTokenManager("tok", ""))
	p.SetBaseURL(server.URL)

	err := p.Authenticate(context.Background(), Credentials{PrimaryToken: "test-token"})
	if err == nil {
		t.Fatal("expected error for forbidden response")
	}
}

func TestGitFlicProvider_ListRepositories_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		repos := []map[string]interface{}{
			{
				"id":          1,
				"title":       "Test Repo",
				"alias":       "test-repo",
				"description": "desc",
				"owner":       "owner1",
				"ownerAlias":  "owner1",
				"httpUrl":     "https://gitflic.ru/owner1/test-repo",
				"sshUrl":      "git@gitflic.ru:owner1/test-repo.git",
				"stars":       5,
				"forks":       2,
				"language":    "Go",
			},
		}
		json.NewEncoder(w).Encode(repos)
	}))
	defer server.Close()

	p := NewGitFlicProvider(NewTokenManager("tok", ""))
	p.SetBaseURL(server.URL)

	repos, err := p.ListRepositories(context.Background(), "org1", ListOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}
	if repos[0].Name != "test-repo" {
		t.Errorf("expected test-repo, got %s", repos[0].Name)
	}
}

func TestGitFlicProvider_ListRepositories_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	p := NewGitFlicProvider(NewTokenManager("tok", ""))
	p.SetBaseURL(server.URL)

	_, err := p.ListRepositories(context.Background(), "org1", ListOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGitFlicProvider_ListRepositories_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	p := NewGitFlicProvider(NewTokenManager("tok", ""))
	p.SetBaseURL(server.URL)

	_, err := p.ListRepositories(context.Background(), "org1", ListOptions{})
	if err == nil {
		t.Fatal("expected error for rate limit")
	}
}

func TestGitFlicProvider_ListRepositories_BadJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	p := NewGitFlicProvider(NewTokenManager("tok", ""))
	p.SetBaseURL(server.URL)

	_, err := p.ListRepositories(context.Background(), "org1", ListOptions{})
	if err == nil {
		t.Fatal("expected error for bad json")
	}
}

func TestGitFlicProvider_ListRepositories_BadStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	p := NewGitFlicProvider(NewTokenManager("tok", ""))
	p.SetBaseURL(server.URL)

	_, err := p.ListRepositories(context.Background(), "org1", ListOptions{})
	if err == nil {
		t.Fatal("expected error for bad status")
	}
}

func TestGitFlicProvider_ListRepositories_Pagination(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.Header().Set("X-Total-Pages", "2")
		}
		repos := []map[string]interface{}{
			{"id": callCount, "alias": "repo", "ownerAlias": "owner"},
		}
		json.NewEncoder(w).Encode(repos)
	}))
	defer server.Close()

	p := NewGitFlicProvider(NewTokenManager("tok", ""))
	p.SetBaseURL(server.URL)

	repos, err := p.ListRepositories(context.Background(), "org1", ListOptions{PerPage: 1, Page: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("expected 2 repos (paginated), got %d", len(repos))
	}
}

func TestGitFlicProvider_ListRepositories_EmptyOrg_UserRepos(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user/repos" {
			t.Errorf("expected /user/repos path, got %s", r.URL.Path)
		}
		repos := []map[string]interface{}{
			{
				"id":          1,
				"title":       "My Repo",
				"alias":       "my-repo",
				"description": "user repo",
				"owner":       "testuser",
				"ownerAlias":  "testuser",
				"httpUrl":     "https://gitflic.ru/testuser/my-repo",
				"sshUrl":      "git@gitflic.ru:testuser/my-repo.git",
				"stars":       3,
				"forks":       1,
				"language":    "Go",
			},
		}
		json.NewEncoder(w).Encode(repos)
	}))
	defer server.Close()

	p := NewGitFlicProvider(NewTokenManager("tok", ""))
	p.SetBaseURL(server.URL)

	repos, err := p.ListRepositories(context.Background(), "", ListOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}
	if repos[0].Name != "my-repo" {
		t.Errorf("expected my-repo, got %s", repos[0].Name)
	}
}

func TestGitFlicProvider_GetRepositoryMetadata_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":          1,
			"title":       "Test Repo",
			"alias":       "test-repo",
			"description": "updated desc",
			"ownerAlias":  "owner1",
			"httpUrl":     "https://gitflic.ru/owner1/test-repo",
			"sshUrl":      "git@gitflic.ru:owner1/test-repo.git",
			"stars":       10,
			"forks":       3,
			"language":    "Go",
		})
	}))
	defer server.Close()

	p := NewGitFlicProvider(NewTokenManager("tok", ""))
	p.SetBaseURL(server.URL)

	repo := models.Repository{Owner: "owner1", Name: "test-repo"}
	result, err := p.GetRepositoryMetadata(context.Background(), repo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Stars != 10 {
		t.Errorf("expected 10 stars, got %d", result.Stars)
	}
}

func TestGitFlicProvider_GetRepositoryMetadata_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	p := NewGitFlicProvider(NewTokenManager("tok", ""))
	p.SetBaseURL(server.URL)

	repo := models.Repository{Owner: "owner1", Name: "test-repo"}
	_, err := p.GetRepositoryMetadata(context.Background(), repo)
	if err == nil {
		t.Fatal("expected error")
	}
}

// ---- GitVerse ----

func TestGitVerseProvider_Authenticate_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	p := NewGitVerseProvider(nil)
	p.baseURL = server.URL

	err := p.Authenticate(context.Background(), Credentials{PrimaryToken: "test-token"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestGitVerseProvider_Authenticate_EmptyToken(t *testing.T) {
	p := NewGitVerseProvider(nil)
	err := p.Authenticate(context.Background(), Credentials{})
	if err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestGitVerseProvider_ListRepositories_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		repos := []map[string]interface{}{
			{
				"id":               1,
				"name":             "test-repo",
				"full_name":        "owner1/test-repo",
				"description":      "desc",
				"html_url":         "https://gitverse.ru/owner1/test-repo",
				"ssh_url":          "git@gitverse.ru:owner1/test-repo.git",
				"stargazers_count": 5,
				"forks_count":      2,
				"language":         "Go",
				"topics":           []string{"go", "test"},
				"archived":         false,
			},
		}
		json.NewEncoder(w).Encode(repos)
	}))
	defer server.Close()

	p := NewGitVerseProvider(NewTokenManager("tok", ""))
	p.baseURL = server.URL

	repos, err := p.ListRepositories(context.Background(), "org1", ListOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}
	if repos[0].Name != "test-repo" {
		t.Errorf("expected test-repo, got %s", repos[0].Name)
	}
	if repos[0].Owner != "owner1" {
		t.Errorf("expected owner1, got %s", repos[0].Owner)
	}
}

func TestGitVerseProvider_ListRepositories_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	p := NewGitVerseProvider(NewTokenManager("tok", ""))
	p.baseURL = server.URL

	_, err := p.ListRepositories(context.Background(), "org1", ListOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGitVerseProvider_ListRepositories_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	p := NewGitVerseProvider(NewTokenManager("tok", ""))
	p.baseURL = server.URL

	_, err := p.ListRepositories(context.Background(), "org1", ListOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGitVerseProvider_ListRepositories_BadStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	p := NewGitVerseProvider(NewTokenManager("tok", ""))
	p.baseURL = server.URL

	_, err := p.ListRepositories(context.Background(), "org1", ListOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGitVerseProvider_ListRepositories_BadJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	p := NewGitVerseProvider(NewTokenManager("tok", ""))
	p.baseURL = server.URL

	_, err := p.ListRepositories(context.Background(), "org1", ListOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGitVerseProvider_GetRepositoryMetadata_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":               1,
			"name":             "test-repo",
			"full_name":        "owner1/test-repo",
			"description":      "updated desc",
			"html_url":         "https://gitverse.ru/owner1/test-repo",
			"ssh_url":          "git@gitverse.ru:owner1/test-repo.git",
			"stargazers_count": 10,
			"forks_count":      3,
			"language":         "Go",
		})
	}))
	defer server.Close()

	p := NewGitVerseProvider(NewTokenManager("tok", ""))
	p.baseURL = server.URL

	repo := models.Repository{Owner: "owner1", Name: "test-repo"}
	result, err := p.GetRepositoryMetadata(context.Background(), repo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Stars != 10 {
		t.Errorf("expected 10 stars, got %d", result.Stars)
	}
}

func TestGitVerseProvider_GetRepositoryMetadata_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	p := NewGitVerseProvider(NewTokenManager("tok", ""))
	p.baseURL = server.URL

	repo := models.Repository{Owner: "owner1", Name: "test-repo"}
	result, err := p.GetRepositoryMetadata(context.Background(), repo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsArchived {
		t.Error("expected archived=true for 404")
	}
}

func TestGitVerseProvider_GetRepositoryMetadata_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	p := NewGitVerseProvider(NewTokenManager("tok", ""))
	p.baseURL = server.URL

	repo := models.Repository{Owner: "owner1", Name: "test-repo"}
	_, err := p.GetRepositoryMetadata(context.Background(), repo)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGitVerseProvider_DetectCapabilities(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/topics" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	p := NewGitVerseProvider(NewTokenManager("tok", ""))
	p.baseURL = server.URL
	p.detectCapabilities(context.Background())

	if !p.capabilities["topics"] {
		t.Error("expected topics capability to be true")
	}
	if p.capabilities["templates"] {
		t.Error("expected templates capability to be false")
	}
}

// ---- GitHub SetBaseURL error ----

func TestGitHubProvider_SetBaseURL_Error(t *testing.T) {
	p := NewGitHubProvider(nil)
	err := p.SetBaseURL("://invalid")
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestGitHubProvider_SetBaseURL_NilClient(t *testing.T) {
	p := &GitHubProvider{}
	err := p.SetBaseURL("https://github.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---- GitLab SetBaseURL error ----

func TestGitLabProvider_SetBaseURL_Error(t *testing.T) {
	p := NewGitLabProvider(nil, "")
	err := p.SetBaseURL("://invalid")
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

// ---- doRequest error paths ----

func TestGitFlicProvider_DoRequest_ClosedServer(t *testing.T) {
	p := NewGitFlicProvider(NewTokenManager("tok", ""))
	p.SetBaseURL("http://localhost:1")

	req, _ := http.NewRequest("GET", "http://localhost:1/test", nil)
	_, err := p.doRequest(req)
	if err == nil {
		t.Fatal("expected error for closed server")
	}
}

func TestGitVerseProvider_DoRequest_ClosedServer(t *testing.T) {
	p := NewGitVerseProvider(NewTokenManager("tok", ""))
	p.baseURL = "http://localhost:1"

	req, _ := http.NewRequest("GET", "http://localhost:1/test", nil)
	_, err := p.doRequest(req)
	if err == nil {
		t.Fatal("expected error for closed server")
	}
}
