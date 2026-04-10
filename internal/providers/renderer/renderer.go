package renderer

import (
	"context"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

type MirrorURL struct {
	Service string
	URL     string
	Label   string
}

type RenderOptions struct {
	TierMapping map[string]string
	MirrorURLs  []MirrorURL
}

type FormatRenderer interface {
	Format() string
	Render(ctx context.Context, content models.Content, opts RenderOptions) ([]byte, error)
	SupportedContentTypes() []string
}
