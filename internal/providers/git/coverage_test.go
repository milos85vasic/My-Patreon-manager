package git

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-github/v69/github"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/xanzy/go-gitlab"
)

// --- Provider Name tests ---

func TestGitHubProvider_Name(t *testing.T) {
	p := NewGitHubProvider(nil)
	if p.Name() != "github" {
		t.Errorf("expected 'github', got %q", p.Name())
	}
}

func TestGitLabProvider_Name(t *testing.T) {
	p := NewGitLabProvider(nil, "")
	if p.Name() != "gitlab" {
		t.Errorf("expected 'gitlab', got %q", p.Name())
	}
}

func TestGitFlicProvider_Name(t *testing.T) {
	p := NewGitFlicProvider(nil)
	if p.Name() != "gitflic" {
		t.Errorf("expected 'gitflic', got %q", p.Name())
	}
}

func TestGitVerseProvider_Name(t *testing.T) {
	p := NewGitVerseProvider(nil)
	if p.Name() != "gitverse" {
		t.Errorf("expected 'gitverse', got %q", p.Name())
	}
}

// --- CheckRepositoryState tests ---

func TestGitFlicProvider_CheckRepositoryState(t *testing.T) {
	p := NewGitFlicProvider(nil)
	repo := models.Repository{ID: "r1"}
	state, err := p.CheckRepositoryState(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if state.RepositoryID != "r1" {
		t.Errorf("expected r1, got %s", state.RepositoryID)
	}
}

func TestGitVerseProvider_CheckRepositoryState(t *testing.T) {
	p := NewGitVerseProvider(nil)
	repo := models.Repository{ID: "r1"}
	state, err := p.CheckRepositoryState(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if state.RepositoryID != "r1" {
		t.Errorf("expected r1, got %s", state.RepositoryID)
	}
}

// GitHub and GitLab CheckRepositoryState make real API calls
// and are covered by integration tests, not unit tests.

// --- DetectMirrors tests ---

func TestGitFlicProvider_DetectMirrors(t *testing.T) {
	p := NewGitFlicProvider(nil)
	repos := []models.Repository{{ID: "r1", Service: "gitflic"}}
	mirrors, err := p.DetectMirrors(context.Background(), repos)
	if err != nil {
		t.Fatal(err)
	}
	_ = mirrors
}

func TestGitVerseProvider_DetectMirrors(t *testing.T) {
	p := NewGitVerseProvider(nil)
	repos := []models.Repository{{ID: "r1", Service: "gitverse"}}
	mirrors, err := p.DetectMirrors(context.Background(), repos)
	if err != nil {
		t.Fatal(err)
	}
	_ = mirrors
}

func TestGitHubProvider_DetectMirrors(t *testing.T) {
	p := NewGitHubProvider(nil)
	repos := []models.Repository{{ID: "r1", Service: "github"}}
	mirrors, err := p.DetectMirrors(context.Background(), repos)
	if err != nil {
		t.Fatal(err)
	}
	_ = mirrors
}

func TestGitLabProvider_DetectMirrors(t *testing.T) {
	p := NewGitLabProvider(nil, "")
	repos := []models.Repository{{ID: "r1", Service: "gitlab"}}
	mirrors, err := p.DetectMirrors(context.Background(), repos)
	if err != nil {
		t.Fatal(err)
	}
	_ = mirrors
}

// --- splitFullName / splitString (GitVerse) ---

func TestSplitFullName(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"owner/repo", 2},
		{"owner", 1},
		{"a/b/c", 3},
		{"", 0},
	}
	for _, tt := range tests {
		parts := splitFullName(tt.input)
		if len(parts) != tt.expected {
			t.Errorf("splitFullName(%q) = %d parts, want %d", tt.input, len(parts), tt.expected)
		}
	}
}

func TestSplitString(t *testing.T) {
	parts := splitString("a/b/c", "/")
	if len(parts) != 3 {
		t.Errorf("expected 3 parts, got %d", len(parts))
	}
	if parts[0] != "a" || parts[1] != "b" || parts[2] != "c" {
		t.Errorf("unexpected parts: %v", parts)
	}

	parts = splitString("single", "/")
	if len(parts) != 1 || parts[0] != "single" {
		t.Errorf("expected ['single'], got %v", parts)
	}
}

// --- MirrorDetector tests ---

func TestMirrorDetector_DetectMirrors_SameNameDifferentService(t *testing.T) {
	detector := NewMirrorDetector()
	repos := []models.Repository{
		{ID: "r1", Service: "github", Owner: "owner", Name: "repo", READMEContent: "hello", LastCommitSHA: "abc123"},
		{ID: "r2", Service: "gitlab", Owner: "owner", Name: "repo", READMEContent: "hello", LastCommitSHA: "abc123"},
	}
	mirrors := detector.DetectMirrors(repos)
	if len(mirrors) != 2 {
		t.Errorf("expected 2 mirror maps, got %d", len(mirrors))
	}
}

func TestMirrorDetector_DetectMirrors_NoMirror(t *testing.T) {
	detector := NewMirrorDetector()
	repos := []models.Repository{
		{ID: "r1", Service: "github", Owner: "owner1", Name: "repo1"},
		{ID: "r2", Service: "gitlab", Owner: "owner2", Name: "repo2"},
	}
	mirrors := detector.DetectMirrors(repos)
	if len(mirrors) != 0 {
		t.Errorf("expected 0 mirror maps, got %d", len(mirrors))
	}
}

func TestMirrorDetector_ComputeSimilarity(t *testing.T) {
	detector := NewMirrorDetector()

	// Same name and owner
	r1 := models.Repository{Service: "github", Owner: "Owner", Name: "Repo"}
	r2 := models.Repository{Service: "gitlab", Owner: "owner", Name: "repo"}
	score := detector.computeSimilarity(r1, r2)
	if score < 0.5 {
		t.Errorf("expected high similarity for same name/owner, got %f", score)
	}

	// Same name, different owner
	r3 := models.Repository{Service: "gitlab", Owner: "other", Name: "repo"}
	score = detector.computeSimilarity(r1, r3)
	if score < 0.5 {
		t.Errorf("expected >= 0.5 for same name, got %f", score)
	}

	// Same README hash
	r4 := models.Repository{Service: "github", Owner: "a", Name: "x", READMEContent: "hello"}
	r5 := models.Repository{Service: "gitlab", Owner: "b", Name: "y", READMEContent: "hello"}
	score = detector.computeSimilarity(r4, r5)
	if score < 0.3 {
		t.Errorf("expected >= 0.3 for same README, got %f", score)
	}

	// Same commit SHA
	r6 := models.Repository{Service: "github", Owner: "a", Name: "x", LastCommitSHA: "abc123"}
	r7 := models.Repository{Service: "gitlab", Owner: "b", Name: "y", LastCommitSHA: "abc123"}
	score = detector.computeSimilarity(r6, r7)
	if score < 0.5 {
		t.Errorf("expected >= 0.5 for same commit SHA, got %f", score)
	}

	// Description similarity
	r8 := models.Repository{Service: "github", Owner: "a", Name: "x", Description: "a test repo for testing things"}
	r9 := models.Repository{Service: "gitlab", Owner: "b", Name: "y", Description: "a test repo for testing things"}
	score = detector.computeSimilarity(r8, r9)
	// exact same description gives Jaccard=1.0, so 0.2*1.0=0.2 contribution
}

func TestMirrorDetector_SelectCanonical(t *testing.T) {
	detector := NewMirrorDetector()

	// GitHub preferred over GitLab
	r1 := models.Repository{ID: "r1", Service: "github"}
	r2 := models.Repository{ID: "r2", Service: "gitlab"}
	canonical := detector.selectCanonical(r1, r2)
	if canonical != "r1" {
		t.Errorf("expected r1 (github), got %s", canonical)
	}

	// Same service, earlier creation date
	now := time.Now()
	r3 := models.Repository{ID: "r3", Service: "github", CreatedAt: now.Add(-24 * time.Hour)}
	r4 := models.Repository{ID: "r4", Service: "github", CreatedAt: now}
	canonical = detector.selectCanonical(r3, r4)
	if canonical != "r3" {
		t.Errorf("expected r3 (earlier), got %s", canonical)
	}

	// Same service, same creation (defaults to r2)
	r5 := models.Repository{ID: "r5", Service: "github"}
	r6 := models.Repository{ID: "r6", Service: "github"}
	canonical = detector.selectCanonical(r5, r6)
	if canonical != "r6" {
		t.Errorf("expected r6 (default), got %s", canonical)
	}

	// Unknown service
	r7 := models.Repository{ID: "r7", Service: "unknown"}
	r8 := models.Repository{ID: "r8", Service: "github"}
	canonical = detector.selectCanonical(r7, r8)
	if canonical != "r8" {
		t.Errorf("expected r8 (github over unknown), got %s", canonical)
	}
}

func TestDetectMirrors_PackageLevel(t *testing.T) {
	repos := []models.Repository{
		{ID: "r1", Service: "github", Owner: "owner", Name: "repo"},
	}
	mirrors, err := DetectMirrors(context.Background(), repos)
	if err != nil {
		t.Fatal(err)
	}
	_ = mirrors
}

// --- GitHubProvider toRepository ---

func TestGitHubProvider_ToRepository(t *testing.T) {
	p := NewGitHubProvider(nil)
	name := "test-repo"
	fullName := "owner/test-repo"
	desc := "A test repository"
	stars := 42
	forks := 7
	lang := "Go"
	htmlURL := "https://github.com/owner/test-repo"
	archived := false
	topics := []string{"go", "test"}
	created := time.Now()
	updated := time.Now()

	ghRepo := &github.Repository{
		Name:            &name,
		FullName:        &fullName,
		Description:     &desc,
		StargazersCount: &stars,
		ForksCount:      &forks,
		Language:        &lang,
		HTMLURL:         &htmlURL,
		Archived:        &archived,
		Topics:          topics,
		CreatedAt:       &github.Timestamp{Time: created},
		UpdatedAt:       &github.Timestamp{Time: updated},
	}

	repo := p.toRepository(ghRepo)
	if repo.Name != "test-repo" {
		t.Errorf("expected 'test-repo', got %q", repo.Name)
	}
	if repo.Service != "github" {
		t.Errorf("expected 'github', got %q", repo.Service)
	}
	if repo.Stars != 42 {
		t.Errorf("expected 42 stars, got %d", repo.Stars)
	}
}

// --- GitLabProvider projectToRepo ---

func TestGitLabProvider_ProjectToRepo(t *testing.T) {
	p := NewGitLabProvider(nil, "")
	now := time.Now()
	proj := &gitlab.Project{
		Path:            "repo",
		SSHURLToRepo:    "git@gitlab.com:owner/repo.git",
		HTTPURLToRepo:   "https://gitlab.com/owner/repo",
		Description:     "desc",
		StarCount:       10,
		ForksCount:      5,
		Archived:        false,
		CreatedAt:       &now,
		LastActivityAt:  &now,
		Namespace:       &gitlab.ProjectNamespace{Path: "owner"},
		TagList:         []string{"go", "test"},
	}
	repo := p.projectToRepo(proj)
	if repo.Name != "repo" {
		t.Errorf("expected 'repo', got %q", repo.Name)
	}
	if repo.Service != "gitlab" {
		t.Errorf("expected 'gitlab', got %q", repo.Service)
	}
}

func TestGitLabProvider_SetBaseURL(t *testing.T) {
	p := NewGitLabProvider(nil, "")
	err := p.SetBaseURL("https://custom-gitlab.example.com")
	if err != nil {
		t.Fatal(err)
	}

	// Reset to default
	err = p.SetBaseURL("https://gitlab.com")
	if err != nil {
		t.Fatal(err)
	}

	// Empty (default)
	err = p.SetBaseURL("")
	if err != nil {
		t.Fatal(err)
	}
}
