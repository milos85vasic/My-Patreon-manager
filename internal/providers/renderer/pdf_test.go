package renderer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPDFRenderer_Format(t *testing.T) {
	renderer := NewPDFRenderer()
	assert.Equal(t, "pdf", renderer.Format())
}

func TestPDFRenderer_SupportedContentTypes(t *testing.T) {
	renderer := NewPDFRenderer()
	types := renderer.SupportedContentTypes()
	assert.Equal(t, []string{"application/pdf"}, types)
}

func TestPDFRenderer_Render_BrowserNotFound(t *testing.T) {
	// Temporarily set PATH empty to ensure chromium-browser and google-chrome not found
	t.Setenv("PATH", "")
	renderer := NewPDFRenderer()
	content := models.Content{
		Title: "Test",
		Body:  "Body",
	}
	opts := RenderOptions{}
	result, err := renderer.Render(context.Background(), content, opts)
	require.NoError(t, err)
	// Should return HTML content (fallback)
	output := string(result)
	assert.Contains(t, output, "<!DOCTYPE html>")
	assert.Contains(t, output, "Test")
}

func TestPDFRenderer_Render_ChromiumBrowserFound(t *testing.T) {
	// Create a temporary directory with a dummy chromium-browser executable
	tmpDir := t.TempDir()
	dummyPath := filepath.Join(tmpDir, "chromium-browser")
	err := os.WriteFile(dummyPath, []byte("#!/bin/sh\necho \"dummy\""), 0755)
	require.NoError(t, err)
	// Set PATH to only our temp directory, so google-chrome not found
	t.Setenv("PATH", tmpDir)
	renderer := NewPDFRenderer()
	content := models.Content{
		Title: "Test",
		Body:  "Body",
	}
	opts := RenderOptions{}
	result, err := renderer.Render(context.Background(), content, opts)
	require.NoError(t, err)
	// Should still return HTML content (since PDF generation not implemented)
	output := string(result)
	assert.Contains(t, output, "<!DOCTYPE html>")
}

func TestPDFRenderer_Render_GoogleChromeFound(t *testing.T) {
	// Create a temporary directory with a dummy google-chrome executable
	tmpDir := t.TempDir()
	dummyPath := filepath.Join(tmpDir, "google-chrome")
	err := os.WriteFile(dummyPath, []byte("#!/bin/sh\necho \"dummy\""), 0755)
	require.NoError(t, err)
	// Set PATH to only our temp directory, so chromium-browser not found
	t.Setenv("PATH", tmpDir)
	renderer := NewPDFRenderer()
	content := models.Content{
		Title: "Test",
		Body:  "Body",
	}
	opts := RenderOptions{}
	result, err := renderer.Render(context.Background(), content, opts)
	require.NoError(t, err)
	output := string(result)
	assert.Contains(t, output, "<!DOCTYPE html>")
}

func TestPDFRenderer_Render_HTMLRenderError(t *testing.T) {
	// Temporarily replace newHTMLRenderer with a mock that returns error
	original := newHTMLRenderer
	defer func() { newHTMLRenderer = original }()

	newHTMLRenderer = func() FormatRenderer {
		return &mockRenderer{err: errors.New("mock error")}
	}

	renderer := NewPDFRenderer()
	content := models.Content{
		Title: "Test",
		Body:  "Body",
	}
	opts := RenderOptions{}
	result, err := renderer.Render(context.Background(), content, opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "render html for pdf")
	assert.Nil(t, result)
}

type mockRenderer struct {
	err error
}

func (m *mockRenderer) Format() string                  { return "mock" }
func (m *mockRenderer) SupportedContentTypes() []string { return []string{"text/mock"} }
func (m *mockRenderer) Render(ctx context.Context, content models.Content, opts RenderOptions) ([]byte, error) {
	return nil, m.err
}

func TestPDFRenderer_Render_WithOptions(t *testing.T) {
	renderer := NewPDFRenderer()
	content := models.Content{
		Title: "Test",
		Body:  "# Header",
	}
	opts := RenderOptions{
		TierMapping: map[string]string{"tier": "Gold"},
		MirrorURLs:  []MirrorURL{{Service: "GitHub", URL: "https://example.com", Label: "Source"}},
	}
	result, err := renderer.Render(context.Background(), content, opts)
	require.NoError(t, err)
	output := string(result)
	// HTML should contain converted header
	assert.Contains(t, output, "<h1>Header</h1>")
}
