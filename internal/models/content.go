package models

import (
	"encoding/json"
	"time"
)

type GeneratedContent struct {
	ID                 string    `json:"id" db:"id"`
	RepositoryID       string    `json:"repository_id" db:"repository_id"`
	ContentType        string    `json:"content_type" db:"content_type"`
	Format             string    `json:"format" db:"format"`
	Title              string    `json:"title" db:"title"`
	Body               string    `json:"body" db:"body"`
	QualityScore       float64   `json:"quality_score" db:"quality_score"`
	ModelUsed          string    `json:"model_used" db:"model_used"`
	PromptTemplate     string    `json:"prompt_template" db:"prompt_template"`
	TokenCount         int       `json:"token_count" db:"token_count"`
	GenerationAttempts int       `json:"generation_attempts" db:"generation_attempts"`
	PassedQualityGate  bool      `json:"passed_quality_gate" db:"passed_quality_gate"`
	Status             string    `json:"status" db:"status"`
	CreatedAt          time.Time `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time `json:"updated_at" db:"updated_at"`
}

type ContentTemplate struct {
	ID          string    `json:"id" db:"id"`
	Name        string    `json:"name" db:"name"`
	ContentType string    `json:"content_type" db:"content_type"`
	Language    string    `json:"language" db:"language"`
	Template    string    `json:"template" db:"template"`
	Variables   []string  `json:"variables" db:"variables"`
	MinLength   int       `json:"min_length" db:"min_length"`
	MaxLength   int       `json:"max_length" db:"max_length"`
	QualityTier string    `json:"quality_tier" db:"quality_tier"`
	IsBuiltIn   bool      `json:"is_built_in" db:"is_built_in"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

type Prompt struct {
	TemplateName string            `json:"template_name"`
	Variables    map[string]string `json:"variables"`
	ContentType  string            `json:"content_type"`
}

type GenerationOptions struct {
	ModelID       string        `json:"model_id"`
	MaxTokens     int           `json:"max_tokens"`
	QualityTier   string        `json:"quality_tier"`
	Timeout       time.Duration `json:"timeout"`
	FallbackChain []string      `json:"fallback_chain"`
}

type Content struct {
	Title        string  `json:"title"`
	Body         string  `json:"body"`
	QualityScore float64 `json:"quality_score"`
	ModelUsed    string  `json:"model_used"`
	TokenCount   int     `json:"token_count"`
}

type ModelInfo struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	QualityScore float64       `json:"quality_score"`
	LatencyP95   time.Duration `json:"latency_p95"`
	CostPer1KTok float64       `json:"cost_per_1k_tok"`
}

type UsageStats struct {
	TotalTokens   int64   `json:"total_tokens"`
	EstimatedCost float64 `json:"estimated_cost"`
	BudgetLimit   float64 `json:"budget_limit"`
	BudgetUsedPct float64 `json:"budget_used_pct"`
}

func (t *ContentTemplate) VariablesJSON() string {
	b, _ := json.Marshal(t.Variables)
	return string(b)
}
