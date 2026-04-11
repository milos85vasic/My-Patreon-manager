package content

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/metrics"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/llm"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/renderer"
	"github.com/milos85vasic/My-Patreon-Manager/internal/utils"
)

type Generator struct {
	llm         llm.LLMProvider
	budget      *TokenBudget
	gate        *QualityGate
	store       database.GeneratedContentStore
	metrics     metrics.MetricsCollector
	renderers   []renderer.FormatRenderer
	reviewQueue *ReviewQueue
}

func NewGenerator(
	llmProvider llm.LLMProvider,
	budget *TokenBudget,
	gate *QualityGate,
	store database.GeneratedContentStore,
	m metrics.MetricsCollector,
	renderers []renderer.FormatRenderer,
) *Generator {
	return &Generator{
		llm:       llmProvider,
		budget:    budget,
		gate:      gate,
		store:     store,
		metrics:   m,
		renderers: renderers,
	}
}

func (g *Generator) SetReviewQueue(rq *ReviewQueue) {
	g.reviewQueue = rq
}

func (g *Generator) GenerateForRepository(
	ctx context.Context,
	repo models.Repository,
	templates []models.ContentTemplate,
	mirrorURLs []renderer.MirrorURL,
) (*models.GeneratedContent, error) {

	prompt := g.assemblePrompt(repo, templates)
	opts := models.GenerationOptions{
		ModelID:   "default",
		MaxTokens: 4000,
		Timeout:   30 * time.Second,
	}

	if g.budget != nil {
		if err := g.budget.CheckBudget(opts.MaxTokens); err != nil {
			return nil, fmt.Errorf("token budget insufficient for generation: %w", err)
		}
		if g.metrics != nil {
			g.metrics.SetBudgetUtilization(g.budget.CurrentUtilization())
		}
	}

	maxRetries := 3
	baseDelay := 100 * time.Millisecond
	var content models.Content
	var err error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		content, err = g.llm.GenerateContent(ctx, prompt, opts)
		if err == nil {
			break
		}
		// If context is done, don't retry
		if ctx.Err() != nil {
			return nil, fmt.Errorf("generate content: %w", err)
		}
		// If this is the last attempt, break and return error
		if attempt == maxRetries {
			break
		}
		// Exponential backoff
		delay := time.Duration(1<<uint(attempt)) * baseDelay
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil, fmt.Errorf("generate content: %w", err)
		case <-timer.C:
			// continue retry
		}
	}
	if err != nil {
		return nil, fmt.Errorf("generate content: %w", err)
	}
	if g.metrics != nil {
		g.metrics.RecordLLMTokens(content.ModelUsed, "total", content.TokenCount)
	}

	if g.budget != nil {
		// refund unused tokens if actual usage is less than reserved MaxTokens
		if content.TokenCount < opts.MaxTokens {
			g.budget.Refund(opts.MaxTokens - content.TokenCount)
		}
		if g.metrics != nil {
			g.metrics.SetBudgetUtilization(g.budget.CurrentUtilization())
		}
	}

	passed, score := g.gate.Evaluate(content)
	_ = score

	generated := &models.GeneratedContent{
		ID:                 utils.NewUUID(),
		RepositoryID:       repo.ID,
		ContentType:        "promotional",
		Format:             "markdown",
		Title:              content.Title,
		Body:               content.Body,
		QualityScore:       content.QualityScore,
		ModelUsed:          content.ModelUsed,
		PromptTemplate:     prompt.TemplateName,
		TokenCount:         content.TokenCount,
		GenerationAttempts: 1,
		PassedQualityGate:  passed,
		CreatedAt:          time.Now(),
	}

	for _, r := range g.renderers {
		rendered, err := r.Render(ctx, content, renderer.RenderOptions{
			MirrorURLs: mirrorURLs,
		})
		if err != nil {
			continue
		}
		if r.Format() == "markdown" {
			generated.Body = string(rendered)
		}
	}

	if g.store != nil {
		if err := g.store.Create(ctx, generated); err != nil {
			return generated, fmt.Errorf("store generated content: %w", err)
		}
	}

	if g.metrics != nil {
		qualityTier := "pass"
		if !passed {
			qualityTier = "fail"
		}
		g.metrics.RecordContentGenerated("markdown", qualityTier)
		g.metrics.RecordLLMQualityScore(repo.Name, content.QualityScore)
	}

	return generated, nil
}

func (g *Generator) assemblePrompt(repo models.Repository, templates []models.ContentTemplate) models.Prompt {
	var tmpl string
	var name string
	if len(templates) > 0 {
		tmpl = templates[0].Template
		name = templates[0].Name
	} else {
		tmpl = defaultTemplate
		name = "default"
	}

	variables := map[string]string{
		"REPO_NAME":   repo.Name,
		"REPO_OWNER":  repo.Owner,
		"DESCRIPTION": repo.Description,
		"STAR_COUNT":  fmt.Sprintf("%d", repo.Stars),
		"FORK_COUNT":  fmt.Sprintf("%d", repo.Forks),
		"LANGUAGE":    repo.PrimaryLanguage,
		"TOPICS":      strings.Join(repo.Topics, ", "),
		"SERVICE":     repo.Service,
		"REPO_URL":    repo.HTTPSURL,
	}

	for k, v := range variables {
		tmpl = strings.ReplaceAll(tmpl, "{{"+k+"}}", v)
	}

	return models.Prompt{
		TemplateName: name,
		Variables:    variables,
		ContentType:  "promotional",
	}
}

const defaultTemplate = `# {{REPO_NAME}}

{{DESCRIPTION}}

**Language:** {{LANGUAGE}}
**Stars:** {{STAR_COUNT}} | **Forks:** {{FORK_COUNT}}
**Topics:** {{TOPICS}}

[View on {{SERVICE}}]({{REPO_URL}})
`
