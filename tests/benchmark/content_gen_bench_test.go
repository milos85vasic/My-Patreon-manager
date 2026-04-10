package benchmark

import (
	"context"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/renderer"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/content"
	"github.com/milos85vasic/My-Patreon-Manager/tests/mocks"
)

func BenchmarkPromptAssembly(b *testing.B) {
	repo := models.Repository{
		ID:              "bench-repo",
		Service:         "github",
		Owner:           "owner",
		Name:            "repo",
		Description:     "A benchmark repository for testing prompt assembly performance",
		Stars:           100,
		Forks:           20,
		PrimaryLanguage: "Go",
		HTTPSURL:        "https://github.com/owner/repo",
		Topics:          []string{"go", "benchmark", "testing"},
	}

	templates := []models.ContentTemplate{
		{
			Name:     "benchmark",
			Template: "# {{REPO_NAME}}\n\n{{DESCRIPTION}}\n\n**Language:** {{LANGUAGE}}\n**Stars:** {{STAR_COUNT}} | **Forks:** {{FORK_COUNT}}\n**Topics:** {{TOPICS}}\n\n[View on {{SERVICE}}]({{REPO_URL}})",
		},
	}

	// Use a minimal generator to access assemblePrompt (private). We'll create a generator with nil dependencies.
	llmMock := &mocks.MockLLMProvider{
		GenerateContentFunc: func(_ context.Context, _ models.Prompt, _ models.GenerationOptions) (models.Content, error) {
			return models.Content{}, nil
		},
	}
	budget := content.NewTokenBudget(1000000)
	gate := content.NewQualityGate(0.75)
	gen := content.NewGenerator(llmMock, budget, gate, nil, nil, nil)

	// We cannot directly call assemblePrompt as it's private. Instead, benchmark the public method that uses it,
	// but that includes LLM call. We'll extract the logic from assemblePrompt and replicate it here.
	// However, we can benchmark the whole GenerateForRepository with a mock that returns quickly.
	// Let's do that instead.
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = gen.GenerateForRepository(context.Background(), repo, templates, nil)
	}
}

func BenchmarkLLMCall(b *testing.B) {
	llmMock := &mocks.MockLLMProvider{
		GenerateContentFunc: func(_ context.Context, prompt models.Prompt, _ models.GenerationOptions) (models.Content, error) {
			return models.Content{
				Title:        "Generated",
				Body:         "Body content",
				QualityScore: 0.9,
				ModelUsed:    "gpt-4",
				TokenCount:   100,
			}, nil
		},
	}
	prompt := models.Prompt{
		TemplateName: "test",
		Variables: map[string]string{
			"REPO_NAME":   "repo",
			"REPO_OWNER":  "owner",
			"DESCRIPTION": "desc",
			"STAR_COUNT":  "100",
			"FORK_COUNT":  "20",
			"LANGUAGE":    "Go",
			"TOPICS":      "go",
			"SERVICE":     "github",
			"REPO_URL":    "https://github.com/owner/repo",
		},
		ContentType: "promotional",
	}
	opts := models.GenerationOptions{
		ModelID: "gpt-4",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = llmMock.GenerateContent(context.Background(), prompt, opts)
	}
}

func BenchmarkMarkdownRendering(b *testing.B) {
	mdRenderer := renderer.NewMarkdownRenderer()
	content := models.Content{
		Title:        "Benchmark Content",
		Body:         "# Hello World\n\nThis is a **benchmark** content with [a link](https://example.com).",
		QualityScore: 0.9,
		ModelUsed:    "gpt-4",
		TokenCount:   50,
	}
	opts := renderer.RenderOptions{
		TierMapping: map[string]string{"tier1": "Premium"},
		MirrorURLs: []renderer.MirrorURL{
			{Service: "GitHub", URL: "https://github.com/owner/repo", Label: "Star and follow"},
			{Service: "GitLab", URL: "https://gitlab.com/owner/repo", Label: "Fork and contribute"},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = mdRenderer.Render(context.Background(), content, opts)
	}
}

func BenchmarkQualityEvaluation(b *testing.B) {
	gate := content.NewQualityGate(0.75)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gate.EvaluateQuality("Sample content for quality evaluation", 0.85)
	}
}
