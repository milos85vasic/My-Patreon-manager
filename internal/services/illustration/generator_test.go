package illustration

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	imgprov "github.com/milos85vasic/My-Patreon-Manager/internal/providers/image"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeFile is a tiny helper that writes b to p, creating the parent dir
// if needed. Used by tests to plant blockers on the filesystem.
func writeFile(p string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
}

// mkDirAt creates a directory at p, creating parents. Used to plant a
// directory where the generator expects to write a file (triggers EISDIR).
func mkDirAt(p string) error {
	return os.MkdirAll(p, 0o755)
}

// --- test doubles ---------------------------------------------------------

// stubIllStore is a minimal in-memory implementation of the small subset of
// database.IllustrationStore the generator touches.
type stubIllStore struct {
	existing    *models.Illustration
	lookupErr   error
	createdWith *models.Illustration
	createErr   error
}

func (s *stubIllStore) Create(_ context.Context, ill *models.Illustration) error {
	s.createdWith = ill
	return s.createErr
}

func (s *stubIllStore) GetByID(_ context.Context, _ string) (*models.Illustration, error) {
	return nil, nil
}

func (s *stubIllStore) GetByContentID(_ context.Context, _ string) (*models.Illustration, error) {
	return nil, nil
}

func (s *stubIllStore) GetByFingerprint(_ context.Context, _ string) (*models.Illustration, error) {
	return s.existing, s.lookupErr
}

func (s *stubIllStore) ListByRepository(_ context.Context, _ string) ([]*models.Illustration, error) {
	return nil, nil
}

func (s *stubIllStore) Delete(_ context.Context, _ string) error { return nil }

// stubImgProvider is a single-provider ImageProvider driven by configurable
// result/err fields so the generator's happy-path and failure-path branches
// can be exercised without touching any real image API.
type stubImgProvider struct {
	name      string
	available bool
	result    *imgprov.ImageResult
	err       error
}

func (s *stubImgProvider) ProviderName() string                 { return s.name }
func (s *stubImgProvider) IsAvailable(_ context.Context) bool   { return s.available }
func (s *stubImgProvider) GenerateImage(_ context.Context, _ imgprov.ImageRequest) (*imgprov.ImageResult, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.result, nil
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestPromptBuilder_Build(t *testing.T) {
	repo := &models.Repository{
		Name:            "my-go-project",
		Description:     "A scalable API built with Go",
		PrimaryLanguage: "Go",
		Topics:          []string{"api", "microservices"},
	}
	content := &models.GeneratedContent{
		Title: "Building Scalable APIs",
	}

	pb := NewPromptBuilder("modern tech illustration, clean lines")
	prompt := pb.Build(repo, content)
	assert.Contains(t, prompt, "my-go-project")
	assert.Contains(t, prompt, "Go")
	assert.Contains(t, prompt, "modern tech illustration")
}

func TestPromptBuilder_BuildFromFields(t *testing.T) {
	pb := NewPromptBuilder("default style")
	prompt := pb.BuildFromFields("repo-name", "A description", "Rust", []string{"web", "cli"}, "Article Title", "")
	assert.Contains(t, prompt, "repo-name")
	assert.Contains(t, prompt, "Rust")
	assert.Contains(t, prompt, "web, cli")
	assert.Contains(t, prompt, "default style")
}

func TestPromptBuilder_EmptyFields(t *testing.T) {
	pb := NewPromptBuilder("")
	prompt := pb.BuildFromFields("", "", "", nil, "", "")
	assert.Empty(t, prompt)
}

func TestStyleLoader_DefaultStyle(t *testing.T) {
	sl := NewStyleLoader("global default style")
	style := sl.LoadStyle(nil)
	assert.Equal(t, "global default style", style)
}

func TestStyleLoader_RepoOverride(t *testing.T) {
	sl := NewStyleLoader("global default")
	override := "custom repo style"
	style := sl.LoadStyle(&override)
	assert.Equal(t, "custom repo style", style)
}

func TestStyleLoader_EmptyOverride(t *testing.T) {
	sl := NewStyleLoader("global default")
	empty := ""
	style := sl.LoadStyle(&empty)
	assert.Equal(t, "global default", style)
}

// newTestGenerator assembles a Generator wired against in-test stubs. The
// image dir is always under t.TempDir so tests don't leave files behind.
func newTestGenerator(t *testing.T, store *stubIllStore, provider *stubImgProvider) *Generator {
	t.Helper()
	fp := imgprov.NewFallbackProvider(provider)
	fp.SetLogger(discardLogger())
	return NewGenerator(
		fp,
		store,
		NewStyleLoader("test-style"),
		NewPromptBuilder("test-style"),
		discardLogger(),
		t.TempDir(),
	)
}

func TestGenerator_GenerateForRevision_HappyPath(t *testing.T) {
	store := &stubIllStore{}
	prov := &stubImgProvider{
		name:      "stub",
		available: true,
		result: &imgprov.ImageResult{
			Data:     []byte("image-bytes"),
			Format:   "png",
			Provider: "stub",
		},
	}
	g := newTestGenerator(t, store, prov)

	repo := &models.Repository{
		ID:              "r1",
		Name:            "demo",
		Description:     "A tiny project",
		PrimaryLanguage: "Go",
		Topics:          []string{"cli"},
	}
	ill, err := g.GenerateForRevision(context.Background(), repo, "article body")
	require.NoError(t, err)
	require.NotNil(t, ill)
	assert.Equal(t, "r1", ill.RepositoryID)
	assert.Equal(t, "r1", ill.GeneratedContentID, "placeholder should be repo.ID")
	assert.NotEmpty(t, ill.FilePath)
	assert.Equal(t, "png", ill.Format)
	assert.Equal(t, "stub", ill.ProviderUsed)
	assert.NotEmpty(t, ill.Fingerprint)
	assert.NotEmpty(t, ill.ID)
	// File should have been written to the image dir.
	assert.Equal(t, filepath.Dir(ill.FilePath), g.imageDir)
	// Store.Create received the new illustration.
	require.NotNil(t, store.createdWith)
	assert.Equal(t, ill.ID, store.createdWith.ID)
}

func TestGenerator_GenerateForRevision_CacheHit(t *testing.T) {
	cached := &models.Illustration{
		ID:           "ill_cached",
		RepositoryID: "r1",
		FilePath:     "/tmp/already-there.png",
		Fingerprint:  "does-not-matter",
	}
	store := &stubIllStore{existing: cached}
	prov := &stubImgProvider{
		name:      "should-not-be-called",
		available: true,
		err:       errors.New("provider invoked despite cache hit"),
	}
	g := newTestGenerator(t, store, prov)

	repo := &models.Repository{ID: "r1", Name: "demo"}
	ill, err := g.GenerateForRevision(context.Background(), repo, "body")
	require.NoError(t, err)
	require.NotNil(t, ill)
	assert.Same(t, cached, ill, "cache hit should return the stored illustration unchanged")
	assert.Nil(t, store.createdWith, "cache hit must not call Create")
}

func TestGenerator_GenerateForRevision_ProviderFailure_ReturnsNilNil(t *testing.T) {
	store := &stubIllStore{}
	prov := &stubImgProvider{
		name:      "stub",
		available: true,
		err:       errors.New("boom"),
	}
	g := newTestGenerator(t, store, prov)

	repo := &models.Repository{ID: "r1", Name: "demo"}
	ill, err := g.GenerateForRevision(context.Background(), repo, "body")
	assert.NoError(t, err, "provider failure should be fail-soft")
	assert.Nil(t, ill, "fail-soft returns nil illustration")
	assert.Nil(t, store.createdWith, "nothing stored on provider failure")
}

func TestGenerator_GenerateForRevision_NilRepo(t *testing.T) {
	store := &stubIllStore{}
	prov := &stubImgProvider{name: "stub", available: true}
	g := newTestGenerator(t, store, prov)

	_, err := g.GenerateForRevision(context.Background(), nil, "body")
	assert.Error(t, err)
}

// TestGenerator_GenerateForRevision_URLResult exercises the branch where the
// provider returns a URL (no bytes); the filePath falls back to the URL.
func TestGenerator_GenerateForRevision_URLResult(t *testing.T) {
	store := &stubIllStore{}
	prov := &stubImgProvider{
		name:      "stub",
		available: true,
		result: &imgprov.ImageResult{
			URL:      "https://example.com/image.png",
			Format:   "png",
			Provider: "stub",
		},
	}
	g := newTestGenerator(t, store, prov)

	repo := &models.Repository{ID: "r1", Name: "demo"}
	ill, err := g.GenerateForRevision(context.Background(), repo, "body")
	require.NoError(t, err)
	require.NotNil(t, ill)
	assert.Equal(t, "https://example.com/image.png", ill.FilePath)
	assert.Equal(t, "https://example.com/image.png", ill.ImageURL)
}

// TestGenerator_GenerateForRevision_MkdirFail forces MkdirAll to fail by
// pointing the image dir at a path that cannot be created (a path rooted
// inside an existing regular file).
func TestGenerator_GenerateForRevision_MkdirFail(t *testing.T) {
	// Create a regular file; then ask the generator to create a subdir
	// under that file. POSIX filesystems fail with ENOTDIR.
	tmp := t.TempDir()
	blocker := filepath.Join(tmp, "blocker")
	require.NoError(t, writeFile(blocker, []byte("x")))
	badDir := filepath.Join(blocker, "subdir")

	store := &stubIllStore{}
	prov := &stubImgProvider{
		name:      "stub",
		available: true,
		result: &imgprov.ImageResult{
			Data: []byte("bytes"), Format: "png", Provider: "stub",
		},
	}
	fp := imgprov.NewFallbackProvider(prov)
	fp.SetLogger(discardLogger())
	g := NewGenerator(
		fp, store,
		NewStyleLoader("style"),
		NewPromptBuilder("style"),
		discardLogger(),
		badDir,
	)
	ill, err := g.GenerateForRevision(context.Background(), &models.Repository{ID: "r1"}, "body")
	assert.Error(t, err)
	assert.Nil(t, ill)
}

// TestGenerator_GenerateForRevision_WriteFileFail forces WriteFile to fail
// by targeting a read-only parent dir (chmod 0o500 after MkdirAll succeeds
// on the parent the first time).
func TestGenerator_GenerateForRevision_WriteFileFail(t *testing.T) {
	// Pre-create a directory with a child that matches the filename the
	// generator will try to write to — but make that child a directory
	// instead of a regular file. WriteFile then fails with EISDIR.
	dir := t.TempDir()
	store := &stubIllStore{}
	prov := &stubImgProvider{
		name:      "stub",
		available: true,
		result: &imgprov.ImageResult{
			Data: []byte("bytes"), Format: "png", Provider: "stub",
		},
	}
	fp := imgprov.NewFallbackProvider(prov)
	fp.SetLogger(discardLogger())
	g := NewGenerator(
		fp, store,
		NewStyleLoader("style"),
		NewPromptBuilder("style"),
		discardLogger(),
		dir,
	)
	// Compute the expected filename the generator will try to write, then
	// pre-create a directory at that path so WriteFile fails.
	hash := computeContentHash([]byte("bytes"))
	target := filepath.Join(dir, hash+".png")
	require.NoError(t, mkDirAt(target))

	ill, err := g.GenerateForRevision(context.Background(), &models.Repository{ID: "r1"}, "body")
	assert.Error(t, err)
	assert.Nil(t, ill)
}

// TestGenerator_GenerateForRevision_StoreCreateError ensures that when
// Store.Create fails the in-memory illustration is still returned so the
// pipeline can proceed; this matches the documented fail-soft behavior.
func TestGenerator_GenerateForRevision_StoreCreateError(t *testing.T) {
	store := &stubIllStore{createErr: errors.New("db-down")}
	prov := &stubImgProvider{
		name:      "stub",
		available: true,
		result: &imgprov.ImageResult{
			Data:     []byte("bytes"),
			Format:   "png",
			Provider: "stub",
		},
	}
	g := newTestGenerator(t, store, prov)

	repo := &models.Repository{ID: "r1", Name: "demo"}
	ill, err := g.GenerateForRevision(context.Background(), repo, "body")
	require.NoError(t, err)
	require.NotNil(t, ill)
	assert.NotEmpty(t, ill.ID, "ID should be populated even if Create fails")
}
