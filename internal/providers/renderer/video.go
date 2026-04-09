package renderer

import (
	"context"
	"fmt"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

type VideoScriptRenderer struct{}

func NewVideoScriptRenderer() *VideoScriptRenderer { return &VideoScriptRenderer{} }

func (r *VideoScriptRenderer) Format() string { return "video_script" }

func (r *VideoScriptRenderer) SupportedContentTypes() []string {
	return []string{"text/x-video-script"}
}

func (r *VideoScriptRenderer) Render(ctx context.Context, content models.Content, opts RenderOptions) ([]byte, error) {
	script := fmt.Sprintf("# Narration Script: %s\n\n%s\n", content.Title, content.Body)
	return []byte(script), nil
}
