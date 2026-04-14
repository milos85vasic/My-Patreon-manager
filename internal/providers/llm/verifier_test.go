package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/metrics"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockMetricsCollector struct {
	llmLatencyCalls   []llmLatencyCall
	llmTokensCalls    []llmTokensCall
	qualityScoreCalls []qualityScoreCall
}

type llmLatencyCall struct {
	model   string
	seconds float64
}

type llmTokensCall struct {
	model     string
	tokenType string
	count     int
}

type qualityScoreCall struct {
	repository string
	score      float64
}

func (m *mockMetricsCollector) RecordSyncDuration(service string, status string, seconds float64) {}
func (m *mockMetricsCollector) RecordReposProcessed(service, action string)                       {}
func (m *mockMetricsCollector) RecordAPIError(service, errorType string)                          {}
func (m *mockMetricsCollector) RecordLLMLatency(model string, seconds float64) {
	m.llmLatencyCalls = append(m.llmLatencyCalls, llmLatencyCall{model, seconds})
}
func (m *mockMetricsCollector) RecordLLMTokens(model, tokenType string, count int) {
	m.llmTokensCalls = append(m.llmTokensCalls, llmTokensCall{model, tokenType, count})
}
func (m *mockMetricsCollector) RecordLLMQualityScore(repository string, score float64) {
	m.qualityScoreCalls = append(m.qualityScoreCalls, qualityScoreCall{repository, score})
}
func (m *mockMetricsCollector) RecordContentGenerated(format, qualityTier string) {}
func (m *mockMetricsCollector) RecordPostCreated(tier string)                     {}
func (m *mockMetricsCollector) RecordPostUpdated(tier string)                     {}
func (m *mockMetricsCollector) RecordWebhookEvent(service, eventType string)      {}
func (m *mockMetricsCollector) SetActiveSyncs(count int)                          {}
func (m *mockMetricsCollector) SetBudgetUtilization(percent float64)              {}

func TestVerifierClient_GenerateContent_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/completions", r.URL.Path)
		assert.Equal(t, "Bearer dummy-api-key", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var reqBody struct {
			Prompt      models.Prompt `json:"prompt"`
			ModelID     string        `json:"model_id"`
			MaxTokens   int           `json:"max_tokens"`
			QualityTier string        `json:"quality_tier"`
		}
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		require.NoError(t, err)
		assert.Equal(t, "test-template", reqBody.Prompt.TemplateName)
		assert.Equal(t, "blog_post", reqBody.Prompt.ContentType)
		assert.Equal(t, "model-123", reqBody.ModelID)
		assert.Equal(t, 1000, reqBody.MaxTokens)
		assert.Equal(t, "premium", reqBody.QualityTier)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"content":       "Generated blog post content",
			"title":         "Blog Post Title",
			"quality_score": 0.92,
			"model_used":    "model-123",
			"token_count":   150,
		})
	}))
	defer server.Close()

	mockMetrics := &mockMetricsCollector{}
	client := NewVerifierClient(server.URL, "dummy-api-key", mockMetrics)

	prompt := models.Prompt{
		TemplateName: "test-template",
		ContentType:  "blog_post",
		Variables:    map[string]string{},
	}
	opts := models.GenerationOptions{
		ModelID:       "model-123",
		MaxTokens:     1000,
		QualityTier:   "premium",
		Timeout:       30 * time.Second,
		FallbackChain: []string{},
	}

	ctx := context.Background()
	content, err := client.GenerateContent(ctx, prompt, opts)
	require.NoError(t, err)
	assert.Equal(t, "Blog Post Title", content.Title)
	assert.Equal(t, "Generated blog post content", content.Body)
	assert.Equal(t, 0.92, content.QualityScore)
	assert.Equal(t, "model-123", content.ModelUsed)
	assert.Equal(t, 150, content.TokenCount)

	// Verify metrics were recorded
	assert.Len(t, mockMetrics.llmLatencyCalls, 1)
	assert.Equal(t, "model-123", mockMetrics.llmLatencyCalls[0].model)
	assert.Greater(t, mockMetrics.llmLatencyCalls[0].seconds, 0.0)
	assert.Len(t, mockMetrics.llmTokensCalls, 1)
	assert.Equal(t, "model-123", mockMetrics.llmTokensCalls[0].model)
	assert.Equal(t, "output", mockMetrics.llmTokensCalls[0].tokenType)
	assert.Equal(t, 150, mockMetrics.llmTokensCalls[0].count)
}

func TestVerifierClient_GenerateContent_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "internal server error"}`))
	}))
	defer server.Close()

	client := NewVerifierClient(server.URL, "dummy-api-key", nil)
	prompt := models.Prompt{}
	opts := models.GenerationOptions{ModelID: "model-123"}
	ctx := context.Background()

	_, err := client.GenerateContent(ctx, prompt, opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "llm server error: 500")
}

func TestVerifierClient_GenerateContent_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := NewVerifierClient(server.URL, "dummy-api-key", nil)
	prompt := models.Prompt{}
	opts := models.GenerationOptions{ModelID: "model-123"}
	ctx := context.Background()

	_, err := client.GenerateContent(ctx, prompt, opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "llm rate limited")
}

func TestVerifierClient_GetAvailableModels_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/models", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []models.ModelInfo{
				{
					ID:           "model-1",
					Name:         "Model One",
					QualityScore: 0.85,
					LatencyP95:   100 * time.Millisecond,
					CostPer1KTok: 0.002,
				},
				{
					ID:           "model-2",
					Name:         "Model Two",
					QualityScore: 0.92,
					LatencyP95:   150 * time.Millisecond,
					CostPer1KTok: 0.003,
				},
			},
		})
	}))
	defer server.Close()

	client := NewVerifierClient(server.URL, "dummy-api-key", nil)
	ctx := context.Background()
	models, err := client.GetAvailableModels(ctx)
	require.NoError(t, err)
	require.Len(t, models, 2)
	assert.Equal(t, "model-1", models[0].ID)
	assert.Equal(t, "Model One", models[0].Name)
	assert.Equal(t, 0.85, models[0].QualityScore)
	assert.Equal(t, 100*time.Millisecond, models[0].LatencyP95)
	assert.Equal(t, 0.002, models[0].CostPer1KTok)
	assert.Equal(t, "model-2", models[1].ID)
}

func TestVerifierClient_GetModelQualityScore_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/models/model-123/score", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"quality_score": 0.88,
		})
	}))
	defer server.Close()

	client := NewVerifierClient(server.URL, "dummy-api-key", nil)
	ctx := context.Background()
	score, err := client.GetModelQualityScore(ctx, "model-123")
	require.NoError(t, err)
	assert.Equal(t, 0.88, score)
}

func TestVerifierClient_GetTokenUsage_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/usage", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(models.UsageStats{
			TotalTokens:   5000,
			EstimatedCost: 12.34,
			BudgetLimit:   1000.0,
			BudgetUsedPct: 1.234,
		})
	}))
	defer server.Close()

	client := NewVerifierClient(server.URL, "dummy-api-key", nil)
	ctx := context.Background()
	stats, err := client.GetTokenUsage(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(5000), stats.TotalTokens)
	assert.Equal(t, 12.34, stats.EstimatedCost)
	assert.Equal(t, 1000.0, stats.BudgetLimit)
	assert.Equal(t, 1.234, stats.BudgetUsedPct)
}

func TestVerifierClient_SetBaseURL(t *testing.T) {
	client := NewVerifierClient("http://original", "dummy", nil)
	assert.Equal(t, "http://original", client.baseURL)
	client.SetBaseURL("http://new")
	assert.Equal(t, "http://new", client.baseURL)
}

func TestVerifierClient_SetTransport(t *testing.T) {
	client := NewVerifierClient("http://test", "dummy", nil)
	transport := &http.Transport{}
	client.SetTransport(transport)
	assert.Equal(t, transport, client.client.Transport)
}

func TestVerifierClient_SetCircuitBreaker(t *testing.T) {
	client := NewVerifierClient("http://test", "dummy", nil)
	cb := metrics.NewCircuitBreaker("test", 1, time.Second, time.Second, nil, nil)
	client.SetCircuitBreaker(cb)
	assert.Equal(t, cb, client.cb)
}
