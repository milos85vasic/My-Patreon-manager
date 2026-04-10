package renderer_test

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/renderer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarkdownRenderer_Format(t *testing.T) {
	r := renderer.NewMarkdownRenderer()
	assert.Equal(t, "markdown", r.Format())
}

func TestMarkdownRenderer_SupportedContentTypes(t *testing.T) {
	r := renderer.NewMarkdownRenderer()
	expected := []string{"text/markdown", "text/x-markdown"}
	assert.Equal(t, expected, r.SupportedContentTypes())
}

func TestMarkdownRenderer_Render_BasicFrontmatter(t *testing.T) {
	r := renderer.NewMarkdownRenderer()
	content := models.Content{
		Title: "My Title",
		Body:  "This is the body.",
	}
	opts := renderer.RenderOptions{}

	result, err := r.Render(context.Background(), content, opts)
	require.NoError(t, err)

	expected := `---
title: "My Title"
generated: true
---

This is the body.`
	assert.Equal(t, expected, string(result))
}

func TestMarkdownRenderer_Render_EmptyTitle(t *testing.T) {
	r := renderer.NewMarkdownRenderer()
	content := models.Content{
		Title: "",
		Body:  "Body only.",
	}
	opts := renderer.RenderOptions{}

	result, err := r.Render(context.Background(), content, opts)
	require.NoError(t, err)

	expected := `---
title: ""
generated: true
---

Body only.`
	assert.Equal(t, expected, string(result))
}

func TestMarkdownRenderer_Render_TierMapping(t *testing.T) {
	r := renderer.NewMarkdownRenderer()
	content := models.Content{
		Title: "Project X",
		Body:  "Content here.",
	}
	opts := renderer.RenderOptions{
		TierMapping: map[string]string{
			"tier1": "Bronze",
			"tier2": "Silver",
		},
	}

	result, err := r.Render(context.Background(), content, opts)
	require.NoError(t, err)

	// Tiers may appear in any order; we need to check that both appear.
	output := string(result)
	assert.Contains(t, output, "title: \"Project X\"")
	assert.Contains(t, output, "generated: true")
	// Check that tiers line exists and contains both Bronze and Silver
	assert.Contains(t, output, "tiers: \"")
	assert.Contains(t, output, "Bronze")
	assert.Contains(t, output, "Silver")
	// Ensure the format is correct: tiers: "Bronze,Silver" or "Silver,Bronze"
	re := regexp.MustCompile(`tiers: "([^"]+)"`)
	matches := re.FindStringSubmatch(output)
	require.NotNil(t, matches, "tiers line not found")
	tiersStr := matches[1]
	tiers := strings.Split(tiersStr, ",")
	require.Len(t, tiers, 2)
	assert.ElementsMatch(t, []string{"Bronze", "Silver"}, tiers)
}

func TestMarkdownRenderer_Render_LintingRemovesEmptyLink(t *testing.T) {
	r := renderer.NewMarkdownRenderer()
	content := models.Content{
		Title: "Test",
		Body:  "This is a [broken]() link.",
	}
	opts := renderer.RenderOptions{}

	result, err := r.Render(context.Background(), content, opts)
	require.NoError(t, err)

	// Expect the empty parentheses to be removed, leaving "[broken]".
	expected := `---
title: "Test"
generated: true
---

This is a [broken] link.`
	assert.Equal(t, expected, string(result))
}

func TestMarkdownRenderer_Render_TemplateVariableInjection(t *testing.T) {
	// Currently applyTemplateVariables is a stub that returns body unchanged.
	// We test that the body passes through unchanged.
	r := renderer.NewMarkdownRenderer()
	content := models.Content{
		Title: "Test",
		Body:  "Hello {{NAME}}!",
	}
	opts := renderer.RenderOptions{}

	result, err := r.Render(context.Background(), content, opts)
	require.NoError(t, err)

	// The placeholder should remain unchanged.
	assert.Contains(t, string(result), "Hello {{NAME}}!")
}

func TestMarkdownRenderer_Render_ContextCancelled(t *testing.T) {
	r := renderer.NewMarkdownRenderer()
	content := models.Content{
		Title: "Test",
		Body:  "Body",
	}
	opts := renderer.RenderOptions{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// Render should still complete (no context usage currently).
	result, err := r.Render(ctx, content, opts)
	require.NoError(t, err)
	assert.NotEmpty(t, result)
}
