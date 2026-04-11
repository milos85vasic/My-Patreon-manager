package renderer

import (
	"context"
	"strings"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarkdownRenderer_Format(t *testing.T) {
	renderer := NewMarkdownRenderer()
	assert.Equal(t, "markdown", renderer.Format())
}

func TestMarkdownRenderer_SupportedContentTypes(t *testing.T) {
	renderer := NewMarkdownRenderer()
	types := renderer.SupportedContentTypes()
	assert.Contains(t, types, "text/markdown")
	assert.Contains(t, types, "text/x-markdown")
	assert.Len(t, types, 2)
}

func TestMarkdownRenderer_Render_EmptyOptions(t *testing.T) {
	renderer := NewMarkdownRenderer()
	content := models.Content{
		Title: "Test Title",
		Body:  "Test body content.",
	}
	opts := RenderOptions{}
	result, err := renderer.Render(context.Background(), content, opts)
	require.NoError(t, err)
	output := string(result)
	assert.Contains(t, output, "title: \"Test Title\"")
	assert.Contains(t, output, "generated: true")
	assert.Contains(t, output, "Test body content.")
	assert.True(t, strings.HasPrefix(output, "---\n"))
}

func TestMarkdownRenderer_Render_WithTierMapping(t *testing.T) {
	renderer := NewMarkdownRenderer()
	content := models.Content{
		Title: "Test",
		Body:  "Body",
	}
	opts := RenderOptions{
		TierMapping: map[string]string{
			"tier1": "Silver",
			"tier2": "Gold",
		},
	}
	result, err := renderer.Render(context.Background(), content, opts)
	require.NoError(t, err)
	output := string(result)
	// Should contain tiers line with Silver,Gold (order not guaranteed)
	assert.True(t, strings.Contains(output, "tiers: \"Silver,Gold\"") || strings.Contains(output, "tiers: \"Gold,Silver\""))
}

func TestMarkdownRenderer_Render_WithMirrorURLs(t *testing.T) {
	renderer := NewMarkdownRenderer()
	content := models.Content{
		Title: "Test",
		Body:  "Body",
	}
	opts := RenderOptions{
		MirrorURLs: []MirrorURL{
			{Service: "GitHub", URL: "https://github.com/example/repo", Label: "Source code"},
			{Service: "GitLab", URL: "https://gitlab.com/example/repo", Label: "Mirror"},
		},
	}
	result, err := renderer.Render(context.Background(), content, opts)
	require.NoError(t, err)
	output := string(result)
	assert.Contains(t, output, "## Get the Code")
	assert.Contains(t, output, "- [GitHub](https://github.com/example/repo) – Source code")
	assert.Contains(t, output, "- [GitLab](https://gitlab.com/example/repo) – Mirror")
}

func TestMarkdownRenderer_Render_LintMarkdown(t *testing.T) {
	renderer := NewMarkdownRenderer()
	content := models.Content{
		Title: "Test",
		Body:  "A [broken link]() should be fixed.",
	}
	opts := RenderOptions{}
	result, err := renderer.Render(context.Background(), content, opts)
	require.NoError(t, err)
	output := string(result)
	// lintMarkdown should replace [broken link]() with [broken link]
	assert.NotContains(t, output, "[broken link]()")
	assert.Contains(t, output, "[broken link]")
}

func TestMarkdownRenderer_Render_ApplyTemplateVariables(t *testing.T) {
	// applyTemplateVariables currently does nothing, but we can still test it doesn't break
	renderer := NewMarkdownRenderer()
	content := models.Content{
		Title: "Test",
		Body:  "Some {{variable}} placeholder.",
	}
	opts := RenderOptions{}
	result, err := renderer.Render(context.Background(), content, opts)
	require.NoError(t, err)
	output := string(result)
	assert.Contains(t, output, "{{variable}}")
}
