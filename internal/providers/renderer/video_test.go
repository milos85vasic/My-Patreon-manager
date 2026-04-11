package renderer

import (
	"context"
	"strings"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVideoScriptRenderer_Format(t *testing.T) {
	renderer := NewVideoScriptRenderer()
	assert.Equal(t, "video_script", renderer.Format())
}

func TestVideoScriptRenderer_SupportedContentTypes(t *testing.T) {
	renderer := NewVideoScriptRenderer()
	types := renderer.SupportedContentTypes()
	assert.Equal(t, []string{"text/x-video-script"}, types)
}

func TestVideoScriptRenderer_Render(t *testing.T) {
	renderer := NewVideoScriptRenderer()
	content := models.Content{
		Title: "My Video",
		Body:  "Narrate this slowly.",
	}
	opts := RenderOptions{}
	result, err := renderer.Render(context.Background(), content, opts)
	require.NoError(t, err)
	output := string(result)
	assert.True(t, strings.HasPrefix(output, "# Narration Script: My Video\n"))
	assert.Contains(t, output, "Narrate this slowly.")
}

func TestVideoScriptRenderer_Render_EmptyBody(t *testing.T) {
	renderer := NewVideoScriptRenderer()
	content := models.Content{
		Title: "Title",
		Body:  "",
	}
	opts := RenderOptions{}
	result, err := renderer.Render(context.Background(), content, opts)
	require.NoError(t, err)
	output := string(result)
	assert.Equal(t, "# Narration Script: Title\n\n\n", output)
}
