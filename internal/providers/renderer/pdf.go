package renderer

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

var newHTMLRenderer func() FormatRenderer = func() FormatRenderer { return NewHTMLRenderer() }

// lookPath is an indirection point so tests can control exec.LookPath results
// without mutating the real PATH.
var lookPath = exec.LookPath

// osMkdirTemp and osWriteFile are indirection points for testing OS error paths.
var osMkdirTemp = os.MkdirTemp
var osWriteFile = os.WriteFile

type PDFRenderer struct{}

func NewPDFRenderer() *PDFRenderer { return &PDFRenderer{} }

func (r *PDFRenderer) Format() string { return "pdf" }

func (r *PDFRenderer) SupportedContentTypes() []string {
	return []string{"application/pdf"}
}

func (r *PDFRenderer) Render(ctx context.Context, content models.Content, opts RenderOptions) ([]byte, error) {
	htmlRenderer := newHTMLRenderer()
	htmlContent, err := htmlRenderer.Render(ctx, content, opts)
	if err != nil {
		return nil, fmt.Errorf("render html for pdf: %w", err)
	}

	// Try to find a browser executable
	var browser string
	if path, err := lookPath("chromium-browser"); err == nil {
		browser = path
	} else if path, err := lookPath("google-chrome"); err == nil {
		browser = path
	} else {
		// No browser found — generate a minimal PDF programmatically
		return minimalPDF(content.Title, string(htmlContent)), nil
	}

	// Create temporary directory for PDF generation
	tmpDir, err := osMkdirTemp("", "patreon-pdf-*")
	if err != nil {
		return minimalPDF(content.Title, string(htmlContent)), nil
	}
	defer os.RemoveAll(tmpDir)

	htmlPath := filepath.Join(tmpDir, "content.html")
	pdfPath := filepath.Join(tmpDir, "output.pdf")

	// Write HTML to file
	if err := osWriteFile(htmlPath, htmlContent, 0644); err != nil {
		return minimalPDF(content.Title, string(htmlContent)), nil
	}

	// Set timeout for PDF generation
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Run browser in headless mode to generate PDF
	cmd := exec.CommandContext(timeoutCtx, browser,
		"--headless",
		"--disable-gpu",
		"--no-sandbox",
		"--print-to-pdf="+pdfPath,
		htmlPath,
	)

	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		// PDF generation failed, produce minimal PDF fallback
		return minimalPDF(content.Title, string(htmlContent)), nil
	}

	// Read generated PDF
	pdfData, err := os.ReadFile(pdfPath)
	if err != nil {
		return minimalPDF(content.Title, string(htmlContent)), nil
	}

	return pdfData, nil
}

// minimalPDF produces a valid PDF-1.4 document containing the given text.
// The document has a single A4 page with the title and stripped HTML body
// rendered as plain text.
func minimalPDF(title, htmlContent string) []byte {
	text := stripHTMLTags(htmlContent)
	// Escape PDF text-string special characters
	text = pdfEscapeString(text)
	title = pdfEscapeString(title)

	// Build the stream content: title in bold, then body text
	stream := fmt.Sprintf("BT /F1 16 Tf 72 750 Td (%s) Tj ET\nBT /F1 12 Tf 72 720 Td (%s) Tj ET",
		title, text)
	streamLen := len(stream)

	// Hand-crafted PDF with proper cross-reference table
	var sb strings.Builder
	offsets := make([]int, 6) // objects 1-5

	// Header
	sb.WriteString("%PDF-1.4\n")

	// Object 1 – Catalog
	offsets[1] = sb.Len()
	sb.WriteString("1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n")

	// Object 2 – Pages
	offsets[2] = sb.Len()
	sb.WriteString("2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n")

	// Object 3 – Page
	offsets[3] = sb.Len()
	sb.WriteString("3 0 obj<</Type/Page/MediaBox[0 0 612 792]/Parent 2 0 R/Contents 4 0 R/Resources<</Font<</F1 5 0 R>>>>>>endobj\n")

	// Object 4 – Content stream
	offsets[4] = sb.Len()
	sb.WriteString(fmt.Sprintf("4 0 obj<</Length %d>>stream\n%s\nendstream endobj\n", streamLen, stream))

	// Object 5 – Font
	offsets[5] = sb.Len()
	sb.WriteString("5 0 obj<</Type/Font/Subtype/Type1/BaseFont/Helvetica>>endobj\n")

	// Cross-reference table
	xrefOffset := sb.Len()
	sb.WriteString("xref\n")
	sb.WriteString("0 6\n")
	sb.WriteString("0000000000 65535 f \n")
	for i := 1; i <= 5; i++ {
		sb.WriteString(fmt.Sprintf("%010d 00000 n \n", offsets[i]))
	}

	// Trailer
	sb.WriteString(fmt.Sprintf("trailer<</Size 6/Root 1 0 R>>\nstartxref\n%d\n%%%%EOF\n", xrefOffset))

	return []byte(sb.String())
}

// stripHTMLTags removes HTML tags from the input string, leaving only text content.
func stripHTMLTags(s string) string {
	re := regexp.MustCompile(`<[^>]*>`)
	result := re.ReplaceAllString(s, "")
	// Collapse multiple whitespace/newlines into single spaces
	ws := regexp.MustCompile(`\s+`)
	result = ws.ReplaceAllString(result, " ")
	return strings.TrimSpace(result)
}

// pdfEscapeString escapes characters that are special in PDF text strings:
// backslash, open-paren, close-paren.
func pdfEscapeString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `(`, `\(`)
	s = strings.ReplaceAll(s, `)`, `\)`)
	return s
}
