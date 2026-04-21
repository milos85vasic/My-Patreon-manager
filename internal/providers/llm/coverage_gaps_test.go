package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithBreakerTimeouts(t *testing.T) {
	provider := &mockProvider{
		generateContentFunc: func(ctx context.Context, prompt models.Prompt, opts models.GenerationOptions) (models.Content, error) {
			return models.Content{QualityScore: 0.95}, nil
		},
	}

	fc := NewFallbackChain(
		[]LLMProvider{provider},
		0.8,
		nil,
		WithBreakerTimeouts(1*time.Second, 500*time.Millisecond),
	)
	ctx := context.Background()
	content, err := fc.GenerateContent(ctx, models.Prompt{}, models.GenerationOptions{})
	require.NoError(t, err)
	assert.Equal(t, 0.95, content.QualityScore)
}

func TestFallbackChain_GetAvailableModels_AllFail(t *testing.T) {
	provider1 := &mockProvider{
		getAvailableModelsFunc: func(ctx context.Context) ([]models.ModelInfo, error) {
			return nil, errors.New("fail 1")
		},
	}
	provider2 := &mockProvider{
		getAvailableModelsFunc: func(ctx context.Context) ([]models.ModelInfo, error) {
			return nil, errors.New("fail 2")
		},
	}

	fc := NewFallbackChain([]LLMProvider{provider1, provider2}, 0.8, nil)
	ctx := context.Background()
	models, err := fc.GetAvailableModels(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "all providers failed to list models")
	assert.Nil(t, models)
}

func TestFallbackChain_GetModelQualityScore_AllFail(t *testing.T) {
	provider1 := &mockProvider{
		getModelQualityScoreFunc: func(ctx context.Context, modelID string) (float64, error) {
			return 0, errors.New("fail 1")
		},
	}
	provider2 := &mockProvider{
		getModelQualityScoreFunc: func(ctx context.Context, modelID string) (float64, error) {
			return 0, errors.New("fail 2")
		},
	}

	fc := NewFallbackChain([]LLMProvider{provider1, provider2}, 0.8, nil)
	ctx := context.Background()
	score, err := fc.GetModelQualityScore(ctx, "model-1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "all providers failed to get quality score")
	assert.Equal(t, 0.0, score)
}

func TestFallbackChain_GenerateContent_WithMetrics(t *testing.T) {
	provider := &mockProvider{
		generateContentFunc: func(ctx context.Context, prompt models.Prompt, opts models.GenerationOptions) (models.Content, error) {
			return models.Content{
				QualityScore: 0.70, // below threshold
				ModelUsed:    "m1",
			}, nil
		},
	}

	mc := &mockMetricsCollector{}
	fc := NewFallbackChain([]LLMProvider{provider}, 0.8, mc)
	ctx := context.Background()
	content, err := fc.GenerateContent(ctx, models.Prompt{}, models.GenerationOptions{})
	require.NoError(t, err)
	assert.Equal(t, 0.70, content.QualityScore)
	assert.Len(t, mc.qualityScoreCalls, 1)
}

func TestVerifierClient_GetAvailableModels_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewVerifierClient(server.URL, "key", nil)
	_, err := client.GetAvailableModels(context.Background())
	assert.Error(t, err)
}

func TestVerifierClient_GetModelQualityScore_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewVerifierClient(server.URL, "key", nil)
	_, err := client.GetModelQualityScore(context.Background(), "m1")
	assert.Error(t, err)
}

func TestVerifierClient_GetTokenUsage_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewVerifierClient(server.URL, "key", nil)
	_, err := client.GetTokenUsage(context.Background())
	assert.Error(t, err)
}

func TestVerifierClient_GetAvailableModels_DecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	client := NewVerifierClient(server.URL, "key", nil)
	_, err := client.GetAvailableModels(context.Background())
	assert.Error(t, err)
}

func TestVerifierClient_GetModelQualityScore_DecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	client := NewVerifierClient(server.URL, "key", nil)
	_, err := client.GetModelQualityScore(context.Background(), "m1")
	assert.Error(t, err)
}

func TestVerifierClient_GetTokenUsage_DecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	client := NewVerifierClient(server.URL, "key", nil)
	_, err := client.GetTokenUsage(context.Background())
	assert.Error(t, err)
}

func TestVerifierClient_GenerateContent_DecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	client := NewVerifierClient(server.URL, "key", nil)
	_, err := client.GenerateContent(context.Background(), models.Prompt{}, models.GenerationOptions{})
	assert.Error(t, err)
}

func TestVerifierClient_GenerateContent_NoAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"content":       "body",
			"title":         "title",
			"quality_score": 0.9,
			"model_used":    "m1",
			"token_count":   10,
		})
	}))
	defer server.Close()

	client := NewVerifierClient(server.URL, "", nil)
	content, err := client.GenerateContent(context.Background(), models.Prompt{}, models.GenerationOptions{})
	assert.NoError(t, err)
	assert.Equal(t, "title", content.Title)
}

func TestVerifierClient_DoRequest_ConnectionError(t *testing.T) {
	// Use a server that immediately closes
	client := NewVerifierClient("http://localhost:1", "key", nil)
	_, err := client.GetTokenUsage(context.Background())
	assert.Error(t, err)
}

func TestVerifierClient_DoRequest_RateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := NewVerifierClient(server.URL, "key", nil)
	_, err := client.GetTokenUsage(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "rate limited")
}

func TestVerifierClient_GetProviders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"count": 2,
			"providers": []map[string]interface{}{
				{"name": "openai", "status": "healthy"},
				{"name": "anthropic", "status": "degraded"},
			},
		})
	}))
	defer server.Close()

	client := NewVerifierClient(server.URL, "key", nil)
	providers, err := client.GetProviders(context.Background())
	require.NoError(t, err)
	assert.Len(t, providers, 2)
	assert.Equal(t, "openai", providers[0].Name)
}

func TestVerifierClient_GetProviders_DecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	client := NewVerifierClient(server.URL, "key", nil)
	_, err := client.GetProviders(context.Background())
	assert.Error(t, err)
}

func TestVerifierClient_GetProviders_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewVerifierClient(server.URL, "key", nil)
	_, err := client.GetProviders(context.Background())
	assert.Error(t, err)
}
