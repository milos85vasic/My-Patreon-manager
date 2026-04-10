package llm_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/errors"
	"github.com/milos85vasic/My-Patreon-Manager/internal/metrics"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockMetricsCollector struct {
	recordedLLMLatency []struct {
		model   string
		seconds float64
	}
	recordedLLMTokens []struct {
		model, tokenType string
		count            int
	}
	recordedLLMQuality []struct {
		repository string
		score      float64
	}
	recordedContentGen []struct{ format, qualityTier string }
	recordedAPIError   []struct{ service, errorType string }
	recordedSync       []struct {
		service, status string
		seconds         float64
	}
	recordedRepos       []struct{ service, action string }
	recordedPostCreated []string
	recordedPostUpdated []string
	recordedWebhook     []struct{ service, eventType string }
	activeSyncs         int
	budgetUtilization   float64
}

func (m *mockMetricsCollector) RecordSyncDuration(service, status string, seconds float64) {
	m.recordedSync = append(m.recordedSync, struct {
		service, status string
		seconds         float64
	}{service, status, seconds})
}
func (m *mockMetricsCollector) RecordReposProcessed(service, action string) {
	m.recordedRepos = append(m.recordedRepos, struct{ service, action string }{service, action})
}
func (m *mockMetricsCollector) RecordAPIError(service, errorType string) {
	m.recordedAPIError = append(m.recordedAPIError, struct{ service, errorType string }{service, errorType})
}
func (m *mockMetricsCollector) RecordLLMLatency(model string, seconds float64) {
	m.recordedLLMLatency = append(m.recordedLLMLatency, struct {
		model   string
		seconds float64
	}{model, seconds})
}
func (m *mockMetricsCollector) RecordLLMTokens(model, tokenType string, count int) {
	m.recordedLLMTokens = append(m.recordedLLMTokens, struct {
		model, tokenType string
		count            int
	}{model, tokenType, count})
}
func (m *mockMetricsCollector) RecordLLMQualityScore(repository string, score float64) {
	m.recordedLLMQuality = append(m.recordedLLMQuality, struct {
		repository string
		score      float64
	}{repository, score})
}
func (m *mockMetricsCollector) RecordContentGenerated(format, qualityTier string) {
	m.recordedContentGen = append(m.recordedContentGen, struct{ format, qualityTier string }{format, qualityTier})
}
func (m *mockMetricsCollector) RecordPostCreated(tier string) {
	m.recordedPostCreated = append(m.recordedPostCreated, tier)
}
func (m *mockMetricsCollector) RecordPostUpdated(tier string) {
	m.recordedPostUpdated = append(m.recordedPostUpdated, tier)
}
func (m *mockMetricsCollector) RecordWebhookEvent(service, eventType string) {
	m.recordedWebhook = append(m.recordedWebhook, struct{ service, eventType string }{service, eventType})
}
func (m *mockMetricsCollector) SetActiveSyncs(count int) {
	m.activeSyncs = count
}
func (m *mockMetricsCollector) SetBudgetUtilization(percent float64) {
	m.budgetUtilization = percent
}

func newTestVerifierClient(t *testing.T, serverURL string) (*llm.VerifierClient, *mockMetricsCollector) {
	t.Helper()
	mockMetrics := &mockMetricsCollector{}
	client := llm.NewVerifierClient(serverURL, "test-api-key", mockMetrics)
	return client, mockMetrics
}

func isTimeout(err error) bool {
	if pe, ok := err.(errors.ProviderError); ok {
		return pe.Code() == "timeout"
	}
	return false
}

func TestVerifierClient_GenerateContent_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/completions", r.URL.Path)
		assert.Equal(t, "Bearer test-api-key", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var body map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&body)
		require.NoError(t, err)
		// prompt should be an object
		promptObj, ok := body["prompt"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "test-template", promptObj["template_name"])
		assert.Equal(t, "markdown", promptObj["content_type"])
		// variables map
		variables, ok := promptObj["variables"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "value1", variables["key1"])
		assert.Equal(t, "value2", variables["key2"])
		assert.Equal(t, "gpt-4", body["model_id"])
		assert.Equal(t, float64(1000), body["max_tokens"])
		assert.Equal(t, "high", body["quality_tier"])

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"content":       "Generated content text",
			"title":         "Generated title",
			"quality_score": 0.95,
			"model_used":    "gpt-4",
			"token_count":   123,
		})
	}))
	defer server.Close()

	client, metrics := newTestVerifierClient(t, server.URL)
	prompt := models.Prompt{
		TemplateName: "test-template",
		Variables:    map[string]string{"key1": "value1", "key2": "value2"},
		ContentType:  "markdown",
	}
	opts := models.GenerationOptions{
		ModelID:     "gpt-4",
		MaxTokens:   1000,
		QualityTier: "high",
	}
	content, err := client.GenerateContent(context.Background(), prompt, opts)
	require.NoError(t, err)
	assert.Equal(t, "Generated title", content.Title)
	assert.Equal(t, "Generated content text", content.Body)
	assert.Equal(t, 0.95, content.QualityScore)
	assert.Equal(t, "gpt-4", content.ModelUsed)
	assert.Equal(t, 123, content.TokenCount)

	// Verify metrics recorded
	require.Len(t, metrics.recordedLLMLatency, 1)
	assert.Equal(t, "gpt-4", metrics.recordedLLMLatency[0].model)
	assert.Greater(t, metrics.recordedLLMLatency[0].seconds, 0.0)
	require.Len(t, metrics.recordedLLMTokens, 1)
	assert.Equal(t, "gpt-4", metrics.recordedLLMTokens[0].model)
	assert.Equal(t, "output", metrics.recordedLLMTokens[0].tokenType)
	assert.Equal(t, 123, metrics.recordedLLMTokens[0].count)
}

func TestVerifierClient_GenerateContent_InvalidToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client, _ := newTestVerifierClient(t, server.URL)
	prompt := models.Prompt{}
	opts := models.GenerationOptions{}
	_, err := client.GenerateContent(context.Background(), prompt, opts)
	assert.Error(t, err)
	// Decode error due to missing JSON body
}

func TestVerifierClient_GenerateContent_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client, _ := newTestVerifierClient(t, server.URL)
	prompt := models.Prompt{}
	opts := models.GenerationOptions{}
	_, err := client.GenerateContent(context.Background(), prompt, opts)
	assert.Error(t, err)
	assert.True(t, errors.IsRateLimited(err))
}

func TestVerifierClient_GenerateContent_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client, _ := newTestVerifierClient(t, server.URL)
	prompt := models.Prompt{}
	opts := models.GenerationOptions{}
	_, err := client.GenerateContent(context.Background(), prompt, opts)
	assert.Error(t, err)
	assert.True(t, isTimeout(err))
}

func TestVerifierClient_GenerateContent_NetworkError(t *testing.T) {
	// Create a client with a transport that returns an error
	client, _ := newTestVerifierClient(t, "http://localhost:0") // invalid address
	client.SetTransport(&errorTransport{})
	prompt := models.Prompt{}
	opts := models.GenerationOptions{}
	_, err := client.GenerateContent(context.Background(), prompt, opts)
	assert.Error(t, err)
	assert.True(t, isTimeout(err))
}

type errorTransport struct{}

func (e *errorTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("simulated network error")
}

func TestVerifierClient_GetAvailableModels_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/models", r.URL.Path)
		assert.Equal(t, "Bearer test-api-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{
					"id":              "gpt-4",
					"name":            "GPT-4",
					"quality_score":   0.95,
					"latency_p95":     500000000, // nanoseconds for 500ms
					"cost_per_1k_tok": 0.03,
				},
				{
					"id":              "claude-3",
					"name":            "Claude 3",
					"quality_score":   0.92,
					"latency_p95":     750000000, // nanoseconds for 750ms
					"cost_per_1k_tok": 0.08,
				},
			},
		})
	}))
	defer server.Close()

	client, _ := newTestVerifierClient(t, server.URL)
	models, err := client.GetAvailableModels(context.Background())
	require.NoError(t, err)
	require.Len(t, models, 2)
	assert.Equal(t, "gpt-4", models[0].ID)
	assert.Equal(t, "GPT-4", models[0].Name)
	assert.Equal(t, 0.95, models[0].QualityScore)
	assert.Equal(t, 500*time.Millisecond, models[0].LatencyP95)
	assert.Equal(t, 0.03, models[0].CostPer1KTok)
}

func TestVerifierClient_GetModelQualityScore_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/models/gpt-4/score", r.URL.Path)
		assert.Equal(t, "Bearer test-api-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"quality_score": 0.92,
		})
	}))
	defer server.Close()

	client, _ := newTestVerifierClient(t, server.URL)
	score, err := client.GetModelQualityScore(context.Background(), "gpt-4")
	require.NoError(t, err)
	assert.Equal(t, 0.92, score)
}

func TestVerifierClient_CircuitBreakerTrip(t *testing.T) {
	// Create a custom circuit breaker with threshold 1
	cb := metrics.NewCircuitBreaker("test", 1, 100*time.Millisecond, 50*time.Millisecond,
		func(name string, reason error) {},
		func(name string) {},
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client, _ := newTestVerifierClient(t, server.URL)
	client.SetCircuitBreaker(cb)

	// First call should fail and trip the breaker
	prompt := models.Prompt{}
	opts := models.GenerationOptions{}
	_, err := client.GenerateContent(context.Background(), prompt, opts)
	assert.Error(t, err)
	assert.True(t, isTimeout(err))

	// Circuit breaker should now be open
	assert.Equal(t, metrics.CircuitOpen, cb.State())

	// Subsequent calls should fail fast with circuit breaker error
	_, err = client.GenerateContent(context.Background(), prompt, opts)
	assert.Error(t, err)
	// The error will be from circuit breaker (gobreaker.ErrOpen)
}

func TestVerifierClient_GetTokenUsage_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/usage", r.URL.Path)
		assert.Equal(t, "Bearer test-api-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"total_tokens":    5000,
			"estimated_cost":  15.5,
			"budget_limit":    100.0,
			"budget_used_pct": 15.5,
		})
	}))
	defer server.Close()

	client, _ := newTestVerifierClient(t, server.URL)
	stats, err := client.GetTokenUsage(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(5000), stats.TotalTokens)
	assert.Equal(t, 15.5, stats.EstimatedCost)
	assert.Equal(t, 100.0, stats.BudgetLimit)
	assert.Equal(t, 15.5, stats.BudgetUsedPct)
}
