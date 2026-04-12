package renderer

import (
	"bytes"
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
	// Override lookPath so no browser is found
	origLookPath := lookPath
	defer func() { lookPath = origLookPath }()
	lookPath = func(file string) (string, error) {
		return "", errors.New("not found")
	}

	renderer := NewPDFRenderer()
	content := models.Content{
		Title: "Test",
		Body:  "Body",
	}
	opts := RenderOptions{}
	result, err := renderer.Render(context.Background(), content, opts)
	require.NoError(t, err)
	// Should return valid PDF, not HTML
	assert.True(t, bytes.HasPrefix(result, []byte("%PDF-")), "expected PDF header, got: %q", string(result[:min(len(result), 20)]))
	assert.Contains(t, string(result), "%%EOF")
}

func TestPDFRenderer_Render_ChromiumBrowserFound(t *testing.T) {
	// Create a temporary directory with a dummy chromium-browser executable
	tmpDir := t.TempDir()
	dummyPath := filepath.Join(tmpDir, "chromium-browser")
	err := os.WriteFile(dummyPath, []byte("#!/bin/sh\nexit 1"), 0755)
	require.NoError(t, err)

	origLookPath := lookPath
	defer func() { lookPath = origLookPath }()
	lookPath = func(file string) (string, error) {
		if file == "chromium-browser" {
			return dummyPath, nil
		}
		return "", errors.New("not found")
	}

	renderer := NewPDFRenderer()
	content := models.Content{
		Title: "Test",
		Body:  "Body",
	}
	opts := RenderOptions{}
	result, err := renderer.Render(context.Background(), content, opts)
	require.NoError(t, err)
	// Browser will fail (dummy script exits 1), should fallback to minimal PDF
	assert.True(t, bytes.HasPrefix(result, []byte("%PDF-")), "expected PDF header")
}

func TestPDFRenderer_Render_GoogleChromeFound(t *testing.T) {
	// Create a temporary directory with a dummy google-chrome executable
	tmpDir := t.TempDir()
	dummyPath := filepath.Join(tmpDir, "google-chrome")
	err := os.WriteFile(dummyPath, []byte("#!/bin/sh\nexit 1"), 0755)
	require.NoError(t, err)

	origLookPath := lookPath
	defer func() { lookPath = origLookPath }()
	lookPath = func(file string) (string, error) {
		if file == "google-chrome" {
			return dummyPath, nil
		}
		return "", errors.New("not found")
	}

	renderer := NewPDFRenderer()
	content := models.Content{
		Title: "Test",
		Body:  "Body",
	}
	opts := RenderOptions{}
	result, err := renderer.Render(context.Background(), content, opts)
	require.NoError(t, err)
	assert.True(t, bytes.HasPrefix(result, []byte("%PDF-")), "expected PDF header")
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
	// No browser available
	origLookPath := lookPath
	defer func() { lookPath = origLookPath }()
	lookPath = func(file string) (string, error) {
		return "", errors.New("not found")
	}

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
	assert.True(t, bytes.HasPrefix(result, []byte("%PDF-")), "expected PDF header")
	// The body text should be in the PDF (stripped of HTML tags)
	assert.Contains(t, string(result), "Header")
}

func TestPDFRenderer_Render_MkdirTempError(t *testing.T) {
	origMkdir := osMkdirTemp
	origLookPath := lookPath
	defer func() { osMkdirTemp = origMkdir; lookPath = origLookPath }()

	lookPath = func(file string) (string, error) {
		if file == "chromium-browser" {
			return "/usr/bin/chromium-browser", nil
		}
		return "", errors.New("not found")
	}
	osMkdirTemp = func(dir, pattern string) (string, error) {
		return "", errors.New("cannot create temp dir")
	}

	renderer := NewPDFRenderer()
	content := models.Content{Title: "Test", Body: "Body"}
	result, err := renderer.Render(context.Background(), content, RenderOptions{})
	require.NoError(t, err)
	assert.True(t, bytes.HasPrefix(result, []byte("%PDF-")), "should fall back to minimal PDF")
}

func TestPDFRenderer_Render_WriteFileError(t *testing.T) {
	origWrite := osWriteFile
	origLookPath := lookPath
	defer func() { osWriteFile = origWrite; lookPath = origLookPath }()

	lookPath = func(file string) (string, error) {
		if file == "chromium-browser" {
			return "/usr/bin/chromium-browser", nil
		}
		return "", errors.New("not found")
	}
	osWriteFile = func(name string, data []byte, perm os.FileMode) error {
		return errors.New("disk full")
	}

	renderer := NewPDFRenderer()
	content := models.Content{Title: "Test", Body: "Body"}
	result, err := renderer.Render(context.Background(), content, RenderOptions{})
	require.NoError(t, err)
	assert.True(t, bytes.HasPrefix(result, []byte("%PDF-")), "should fall back to minimal PDF")
}

func TestPDFRenderer_Render_BrowserSuccessProducesPDF(t *testing.T) {
	origLookPath := lookPath
	defer func() { lookPath = origLookPath }()

	// Create a dummy browser script that writes a fake PDF file
	tmpDir := t.TempDir()
	// The script reads the --print-to-pdf= argument to know where to write
	scriptPath := filepath.Join(tmpDir, "chromium-browser")
	script := `#!/bin/sh
for arg in "$@"; do
  case "$arg" in
    --print-to-pdf=*)
      outpath="${arg#--print-to-pdf=}"
      printf '%%PDF-1.4 fake' > "$outpath"
      ;;
  esac
done
`
	err := os.WriteFile(scriptPath, []byte(script), 0755)
	require.NoError(t, err)

	lookPath = func(file string) (string, error) {
		if file == "chromium-browser" {
			return scriptPath, nil
		}
		return "", errors.New("not found")
	}

	renderer := NewPDFRenderer()
	content := models.Content{Title: "Test", Body: "Body"}
	result, err := renderer.Render(context.Background(), content, RenderOptions{})
	require.NoError(t, err)
	assert.True(t, bytes.HasPrefix(result, []byte("%PDF-")), "should return real PDF from browser")
}

func TestPDFRenderer_Render_BrowserSuccessButReadFails(t *testing.T) {
	origLookPath := lookPath
	defer func() { lookPath = origLookPath }()

	// Create a dummy browser script that exits 0 but does NOT write the PDF file
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "chromium-browser")
	script := "#!/bin/sh\nexit 0\n"
	err := os.WriteFile(scriptPath, []byte(script), 0755)
	require.NoError(t, err)

	lookPath = func(file string) (string, error) {
		if file == "chromium-browser" {
			return scriptPath, nil
		}
		return "", errors.New("not found")
	}

	renderer := NewPDFRenderer()
	content := models.Content{Title: "Test", Body: "Body"}
	result, err := renderer.Render(context.Background(), content, RenderOptions{})
	require.NoError(t, err)
	// ReadFile fails because browser didn't write the file; falls back to minimal PDF
	assert.True(t, bytes.HasPrefix(result, []byte("%PDF-")), "should fall back to minimal PDF")
}

func TestMinimalPDF_ValidStructure(t *testing.T) {
	pdf := minimalPDF("Hello", "<p>World</p>")
	assert.True(t, bytes.HasPrefix(pdf, []byte("%PDF-1.4")))
	assert.Contains(t, string(pdf), "%%EOF")
	assert.Contains(t, string(pdf), "/Type/Catalog")
	assert.Contains(t, string(pdf), "/Type/Page")
	assert.Contains(t, string(pdf), "/BaseFont/Helvetica")
	assert.Contains(t, string(pdf), "Hello")
	assert.Contains(t, string(pdf), "World")
}

func TestMinimalPDF_EscapesSpecialChars(t *testing.T) {
	pdf := minimalPDF("Title (v1)", "<p>Hello (world) \\ end</p>")
	s := string(pdf)
	assert.Contains(t, s, `Title \(v1\)`)
	assert.Contains(t, s, `Hello \(world\) \\ end`)
}

func TestStripHTMLTags(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple", "<p>Hello</p>", "Hello"},
		{"nested", "<div><p>Hello <strong>World</strong></p></div>", "Hello World"},
		{"no tags", "Plain text", "Plain text"},
		{"whitespace collapse", "<p>Hello</p>\n\n<p>World</p>", "Hello World"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripHTMLTags(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPDFEscapeString(t *testing.T) {
	assert.Equal(t, `hello`, pdfEscapeString("hello"))
	assert.Equal(t, `\(test\)`, pdfEscapeString("(test)"))
	assert.Equal(t, `back\\slash`, pdfEscapeString(`back\slash`))
	assert.Equal(t, `a\\b\(c\)`, pdfEscapeString(`a\b(c)`))
}
