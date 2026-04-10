package renderer_test

import (
	"context"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/renderer"
	"github.com/stretchr/testify/assert"
)

func TestMarkdownRenderer_MirrorURLs(t *testing.T) {
	r := renderer.NewMarkdownRenderer()
	content := models.Content{
		Title: "My Project",
		Body:  "This is a great project.",
	}
	opts := renderer.RenderOptions{
		MirrorURLs: []renderer.MirrorURL{
			{Service: "github", URL: "https://github.com/user/repo", Label: "Star and follow on GitHub"},
			{Service: "gitlab", URL: "https://gitlab.com/user/repo", Label: "Contribute on GitLab"},
		},
	}
	out, err := r.Render(context.Background(), content, opts)
	assert.NoError(t, err)
	assert.Contains(t, string(out), "Get the Code")
	assert.Contains(t, string(out), "Star and follow on GitHub")
	assert.Contains(t, string(out), "Contribute on GitLab")
}
