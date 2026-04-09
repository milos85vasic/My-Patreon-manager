package renderer

import (
	"context"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

type RenderOptions struct {
	TierMapping map[string]string
}

type FormatRenderer interface {
	Format() string
	Render(ctx context.Context, content models.Content, opts RenderOptions) ([]byte, error)
	SupportedContentTypes() []string
}
