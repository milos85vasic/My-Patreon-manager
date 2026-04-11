package renderer

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

var newHTMLRenderer func() FormatRenderer = func() FormatRenderer { return NewHTMLRenderer() }

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
	if path, err := exec.LookPath("chromium-browser"); err == nil {
		browser = path
	} else if path, err := exec.LookPath("google-chrome"); err == nil {
		browser = path
	} else {
		// No browser found, return HTML as fallback
		return htmlContent, nil
	}

	// Create temporary directory for PDF generation
	tmpDir, err := os.MkdirTemp("", "patreon-pdf-*")
	if err != nil {
		return htmlContent, nil // fallback on temp dir error
	}
	defer os.RemoveAll(tmpDir)

	htmlPath := filepath.Join(tmpDir, "content.html")
	pdfPath := filepath.Join(tmpDir, "output.pdf")

	// Write HTML to file
	if err := os.WriteFile(htmlPath, htmlContent, 0644); err != nil {
		return htmlContent, nil // fallback
	}

	// Run browser in headless mode to generate PDF
	// Use --no-sandbox for container environments, --disable-gpu for headless
	cmd := exec.CommandContext(ctx, browser,
		"--headless",
		"--disable-gpu",
		"--no-sandbox",
		"--print-to-pdf="+pdfPath,
		htmlPath,
	)

	// Set timeout for PDF generation
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd = exec.CommandContext(timeoutCtx, browser,
		"--headless",
		"--disable-gpu",
		"--no-sandbox",
		"--print-to-pdf="+pdfPath,
		htmlPath,
	)

	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		// PDF generation failed, return HTML fallback
		return htmlContent, nil
	}

	// Read generated PDF
	pdfData, err := os.ReadFile(pdfPath)
	if err != nil {
		return htmlContent, nil // fallback
	}

	return pdfData, nil
}
