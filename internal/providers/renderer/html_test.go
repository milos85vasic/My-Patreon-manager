package renderer

import (
	"context"
	"strings"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTMLRenderer_Format(t *testing.T) {
	renderer := NewHTMLRenderer()
	assert.Equal(t, "html", renderer.Format())
}

func TestHTMLRenderer_SupportedContentTypes(t *testing.T) {
	renderer := NewHTMLRenderer()
	types := renderer.SupportedContentTypes()
	assert.Equal(t, []string{"text/html"}, types)
}

func TestHTMLRenderer_Render_Basic(t *testing.T) {
	renderer := NewHTMLRenderer()
	content := models.Content{
		Title: "Test Title",
		Body:  "Hello world.",
	}
	opts := RenderOptions{}
	result, err := renderer.Render(context.Background(), content, opts)
	require.NoError(t, err)
	output := string(result)
	assert.Contains(t, output, "<title>Test Title</title>")
	assert.Contains(t, output, "<p>Hello world.</p>")
	assert.Contains(t, output, "<!DOCTYPE html>")
	assert.Contains(t, output, "<style>")
	assert.Contains(t, output, "</html>")
}

func TestHTMLRenderer_Render_EscapeTitle(t *testing.T) {
	renderer := NewHTMLRenderer()
	content := models.Content{
		Title: "Test <script>alert('xss')</script>",
		Body:  "Body",
	}
	opts := RenderOptions{}
	result, err := renderer.Render(context.Background(), content, opts)
	require.NoError(t, err)
	output := string(result)
	// Title should be escaped
	assert.Contains(t, output, "&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;")
	assert.NotContains(t, output, "<script>alert('xss')</script>")
}

func TestHTMLRenderer_Render_MarkdownConversion(t *testing.T) {
	renderer := NewHTMLRenderer()
	content := models.Content{
		Title: "Test",
		Body:  "# Header\n**bold** and *italic*\n- list item",
	}
	opts := RenderOptions{}
	result, err := renderer.Render(context.Background(), content, opts)
	require.NoError(t, err)
	output := string(result)
	assert.Contains(t, output, "<h1>Header</h1>")
	assert.Contains(t, output, "<strong>bold</strong>")
	assert.Contains(t, output, "<em>italic</em>")
	assert.Contains(t, output, "<li>list item</li>")
}

func TestHTMLRenderer_Render_SanitizeScripts(t *testing.T) {
	renderer := NewHTMLRenderer()
	content := models.Content{
		Title: "Test",
		Body:  "Normal text <script>alert('evil')</script> and <img src=x onerror=alert(1)>",
	}
	opts := RenderOptions{}
	result, err := renderer.Render(context.Background(), content, opts)
	require.NoError(t, err)
	output := string(result)
	// Script tags should be removed
	assert.NotContains(t, output, "<script>alert('evil')</script>")
	// onerror attribute should be stripped
	assert.NotContains(t, output, "onerror=alert(1)")
	// img tag remains but without onerror
	assert.Contains(t, output, "<img src=x>")
}

func TestHTMLRenderer_Render_EmptyBody(t *testing.T) {
	renderer := NewHTMLRenderer()
	content := models.Content{
		Title: "Test",
		Body:  "",
	}
	opts := RenderOptions{}
	result, err := renderer.Render(context.Background(), content, opts)
	require.NoError(t, err)
	output := string(result)
	// Should still produce valid HTML
	assert.Contains(t, output, "<title>Test</title>")
	assert.Contains(t, output, "</body>")
}

func TestHTMLRenderer_Render_WithMirrorURLsAndTierMapping(t *testing.T) {
	// MirrorURLs and TierMapping are not used in HTML renderer, but ensure they don't break
	renderer := NewHTMLRenderer()
	content := models.Content{
		Title: "Test",
		Body:  "Body",
	}
	opts := RenderOptions{
		TierMapping: map[string]string{"a": "Silver"},
		MirrorURLs:  []MirrorURL{{Service: "GitHub", URL: "https://example.com", Label: "Source"}},
	}
	result, err := renderer.Render(context.Background(), content, opts)
	require.NoError(t, err)
	output := string(result)
	// Should still render body
	assert.Contains(t, output, "Body")
}

func TestHTMLRenderer_MarkdownToHTML_Coverage(t *testing.T) {
	renderer := NewHTMLRenderer()
	// Test raw HTML line (starts with "<")
	content := models.Content{
		Title: "Test",
		Body:  "<h1>Raw Header</h1>\nParagraph",
	}
	opts := RenderOptions{}
	result, err := renderer.Render(context.Background(), content, opts)
	require.NoError(t, err)
	output := string(result)
	// raw HTML line should not be wrapped in <p>
	assert.Contains(t, output, "<h1>Raw Header</h1>")
	assert.NotContains(t, output, "<p><h1>Raw Header</h1></p>")
	// Paragraph line should be wrapped
	assert.Contains(t, output, "<p>Paragraph</p>")

	// Test list with empty line after list
	content2 := models.Content{
		Title: "Test",
		Body:  "- item1\n\nparagraph",
	}
	result2, err := renderer.Render(context.Background(), content2, opts)
	require.NoError(t, err)
	output2 := string(result2)
	// list should be wrapped in <ul> and closed before paragraph
	assert.True(t, strings.Contains(output2, "<ul>") && strings.Contains(output2, "</ul>"))
	assert.Contains(t, output2, "<li>item1</li>")
	assert.Contains(t, output2, "<p>paragraph</p>")

	// Test multiple list items without empty line separation
	content3 := models.Content{
		Title: "Test",
		Body:  "- item1\n- item2\n- item3",
	}
	result3, err := renderer.Render(context.Background(), content3, opts)
	require.NoError(t, err)
	output3 := string(result3)
	// all items in same list
	assert.Contains(t, output3, "<li>item1</li>")
	assert.Contains(t, output3, "<li>item2</li>")
	assert.Contains(t, output3, "<li>item3</li>")
	// only one <ul> and </ul> pair
	assert.Equal(t, 1, strings.Count(output3, "<ul>"))
	assert.Equal(t, 1, strings.Count(output3, "</ul>"))
}
