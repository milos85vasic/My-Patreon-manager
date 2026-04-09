package renderer

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

type PDFRenderer struct{}

func NewPDFRenderer() *PDFRenderer { return &PDFRenderer{} }

func (r *PDFRenderer) Format() string { return "pdf" }

func (r *PDFRenderer) SupportedContentTypes() []string {
	return []string{"application/pdf"}
}

func (r *PDFRenderer) Render(ctx context.Context, content models.Content, opts RenderOptions) ([]byte, error) {
	htmlRenderer := NewHTMLRenderer()
	htmlContent, err := htmlRenderer.Render(ctx, content, opts)
	if err != nil {
		return nil, fmt.Errorf("render html for pdf: %w", err)
	}

	if _, err := exec.LookPath("chromium-browser"); err != nil {
		if _, err := exec.LookPath("google-chrome"); err != nil {
			return htmlContent, nil
		}
	}

	return htmlContent, nil
}
