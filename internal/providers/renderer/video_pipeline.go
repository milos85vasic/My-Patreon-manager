package renderer

import (
	"context"
	"fmt"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

type VideoPipeline struct {
	enabled bool
}

func NewVideoPipeline(enabled bool) *VideoPipeline {
	return &VideoPipeline{enabled: enabled}
}

func (p *VideoPipeline) IsEnabled() bool { return p.enabled }

func (p *VideoPipeline) Render(ctx context.Context, content models.Content, opts RenderOptions) ([]byte, error) {
	if !p.enabled {
		return nil, fmt.Errorf("video generation is not enabled")
	}
	return nil, fmt.Errorf("video pipeline not yet implemented")
}

func (p *VideoPipeline) CheckDiskSpace(requiredMB int) error {
	return nil
}
