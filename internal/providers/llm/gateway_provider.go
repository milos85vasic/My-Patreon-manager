package llm

import (
	"context"
	"fmt"
	"time"

	gw "digital.vasic.llmgateway"
	"github.com/milos85vasic/My-Patreon-Manager/internal/metrics"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

// GatewayProvider implements LLMProvider by routing content generation
// through the LLMGateway (direct provider API calls) while using the
// LLMsVerifier HTTP API for model listing and usage stats.
type GatewayProvider struct {
	gateway  *gw.Gateway
	verifier *VerifierClient
	metrics  metrics.MetricsCollector
	model    string // default model to use if none specified
}

// NewGatewayProvider creates an LLMProvider that uses LLMGateway for
// completions and the VerifierClient for model metadata.
func NewGatewayProvider(gateway *gw.Gateway, verifier *VerifierClient, m metrics.MetricsCollector, defaultModel string) *GatewayProvider {
	if defaultModel == "" {
		defaultModel = "deepseek/deepseek-chat"
	}
	return &GatewayProvider{
		gateway:  gateway,
		verifier: verifier,
		metrics:  m,
		model:    defaultModel,
	}
}

// GenerateContent sends the prompt to an LLM provider via the gateway
// and returns the generated content. The gateway handles provider
// selection, fallback, retry, and circuit breaking.
func (p *GatewayProvider) GenerateContent(ctx context.Context, prompt models.Prompt, opts models.GenerationOptions) (models.Content, error) {
	start := time.Now()

	model := opts.ModelID
	if model == "" {
		model = p.model
	}

	systemMsg := fmt.Sprintf(
		"You are a technical content writer for a Patreon page about software engineering. "+
			"Content type: %s. Template: %s. "+
			"Write high-quality, engaging content based on the provided context.",
		prompt.ContentType, prompt.TemplateName,
	)

	userMsg := "Generate content"
	if len(prompt.Variables) > 0 {
		userMsg = "Generate content for the following:\n"
		for k, v := range prompt.Variables {
			userMsg += fmt.Sprintf("- %s: %s\n", k, v)
		}
	}

	maxTokens := opts.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 2048
	}

	req := &gw.Request{
		Model: model,
		Messages: []gw.Message{
			{Role: "system", Content: systemMsg},
			{Role: "user", Content: userMsg},
		},
		MaxTokens: maxTokens,
	}

	resp, err := p.gateway.Complete(ctx, req)

	latency := time.Since(start).Seconds()
	if p.metrics != nil {
		p.metrics.RecordLLMLatency(model, latency)
	}

	if err != nil {
		return models.Content{}, fmt.Errorf("gateway completion: %w", err)
	}

	content := resp.Content()
	tokenCount := resp.TotalTokens()

	if p.metrics != nil {
		p.metrics.RecordLLMTokens(model, "output", tokenCount)
	}

	// Extract a title from the first line if possible
	title := extractTitle(content)

	// Quality score: use token density as a rough proxy (longer, more detailed = higher)
	qualityScore := estimateQuality(content, tokenCount)

	return models.Content{
		Title:        title,
		Body:         content,
		QualityScore: qualityScore,
		ModelUsed:    resp.Model,
		TokenCount:   tokenCount,
	}, nil
}

// GetAvailableModels delegates to the VerifierClient.
func (p *GatewayProvider) GetAvailableModels(ctx context.Context) ([]models.ModelInfo, error) {
	if p.verifier != nil {
		return p.verifier.GetAvailableModels(ctx)
	}
	return nil, nil
}

// GetModelQualityScore delegates to the VerifierClient.
func (p *GatewayProvider) GetModelQualityScore(ctx context.Context, modelID string) (float64, error) {
	if p.verifier != nil {
		return p.verifier.GetModelQualityScore(ctx, modelID)
	}
	return 0.5, nil
}

// GetTokenUsage delegates to the VerifierClient.
func (p *GatewayProvider) GetTokenUsage(ctx context.Context) (models.UsageStats, error) {
	if p.verifier != nil {
		return p.verifier.GetTokenUsage(ctx)
	}
	return models.UsageStats{}, nil
}

// extractTitle returns the first non-empty line as a title, or a default.
func extractTitle(content string) string {
	for i := 0; i < len(content); i++ {
		if content[i] == '\n' {
			line := content[:i]
			if len(line) > 0 {
				// Strip markdown heading prefix
				for len(line) > 0 && line[0] == '#' {
					line = line[1:]
				}
				if len(line) > 0 && line[0] == ' ' {
					line = line[1:]
				}
				if len(line) > 0 {
					if len(line) > 100 {
						return line[:100]
					}
					return line
				}
			}
			break
		}
	}
	return "Generated Content"
}

// estimateQuality returns a rough quality score (0-1) based on content length.
func estimateQuality(content string, tokens int) float64 {
	if len(content) == 0 {
		return 0.0
	}
	// Longer, more detailed responses generally indicate higher quality
	score := float64(len(content)) / 3000.0
	if score > 1.0 {
		score = 1.0
	}
	if score < 0.3 {
		score = 0.3
	}
	return score
}
