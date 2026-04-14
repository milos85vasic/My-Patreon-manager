package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/errors"
	"github.com/milos85vasic/My-Patreon-Manager/internal/metrics"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

type VerifierClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
	cb      *metrics.CircuitBreaker
	metrics metrics.MetricsCollector
}

func NewVerifierClient(baseURL, apiKey string, m metrics.MetricsCollector) *VerifierClient {
	return &VerifierClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 30 * time.Second},
		cb: metrics.NewCircuitBreaker("llm_verifier", 5, 60*time.Second, 30*time.Second,
			metrics.DefaultOnTrip, metrics.DefaultOnReset),
		metrics: m,
	}
}

func (v *VerifierClient) GenerateContent(ctx context.Context, prompt models.Prompt, opts models.GenerationOptions) (models.Content, error) {
	result, err := v.cb.Execute(func() (interface{}, error) {
		start := time.Now()
		defer func() {
			if v.metrics != nil {
				v.metrics.RecordLLMLatency(opts.ModelID, time.Since(start).Seconds())
			}
		}()

		body := map[string]interface{}{
			"prompt":       prompt,
			"model_id":     opts.ModelID,
			"max_tokens":   opts.MaxTokens,
			"quality_tier": opts.QualityTier,
		}
		resp, err := v.doRequest(ctx, "POST", v.baseURL+"/api/completions", body)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		var result struct {
			Content      string  `json:"content"`
			Title        string  `json:"title"`
			QualityScore float64 `json:"quality_score"`
			ModelUsed    string  `json:"model_used"`
			TokenCount   int     `json:"token_count"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("decode completion response: %w", err)
		}

		if v.metrics != nil {
			v.metrics.RecordLLMTokens(result.ModelUsed, "output", result.TokenCount)
		}

		return models.Content{
			Title:        result.Title,
			Body:         result.Content,
			QualityScore: result.QualityScore,
			ModelUsed:    result.ModelUsed,
			TokenCount:   result.TokenCount,
		}, nil
	})

	if err != nil {
		return models.Content{}, err
	}
	return result.(models.Content), nil
}

func (v *VerifierClient) GetAvailableModels(ctx context.Context) ([]models.ModelInfo, error) {
	result, err := v.cb.Execute(func() (interface{}, error) {
		resp, err := v.doRequest(ctx, "GET", v.baseURL+"/api/models", nil)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		var models struct {
			Data []models.ModelInfo `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&models); err != nil {
			return nil, fmt.Errorf("decode models response: %w", err)
		}
		return models.Data, nil
	})

	if err != nil {
		return nil, err
	}
	return result.([]models.ModelInfo), nil
}

func (v *VerifierClient) GetModelQualityScore(ctx context.Context, modelID string) (float64, error) {
	result, err := v.cb.Execute(func() (interface{}, error) {
		resp, err := v.doRequest(ctx, "GET", v.baseURL+"/api/models/"+modelID+"/score", nil)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		var score struct {
			QualityScore float64 `json:"quality_score"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&score); err != nil {
			return nil, fmt.Errorf("decode quality score: %w", err)
		}
		return score.QualityScore, nil
	})

	if err != nil {
		return 0, err
	}
	return result.(float64), nil
}

func (v *VerifierClient) GetTokenUsage(ctx context.Context) (models.UsageStats, error) {
	resp, err := v.doRequest(ctx, "GET", v.baseURL+"/api/usage", nil)
	if err != nil {
		return models.UsageStats{}, err
	}
	defer resp.Body.Close()

	var stats models.UsageStats
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return models.UsageStats{}, fmt.Errorf("decode usage stats: %w", err)
	}
	return stats, nil
}

func (v *VerifierClient) doRequest(ctx context.Context, method, url string, body interface{}) (*http.Response, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if v.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+v.apiKey)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := v.client.Do(req)
	if err != nil {
		return nil, errors.Timeout(fmt.Sprintf("llm request failed: %v", err))
	}
	if resp.StatusCode >= 500 {
		resp.Body.Close()
		return nil, errors.Timeout(fmt.Sprintf("llm server error: %d", resp.StatusCode))
	}
	if resp.StatusCode == 429 {
		resp.Body.Close()
		return nil, errors.RateLimited("llm rate limited", time.Now().Add(1*time.Minute))
	}
	return resp, nil
}

func (v *VerifierClient) SetBaseURL(url string) {
	v.baseURL = url
}

func (v *VerifierClient) SetTransport(rt http.RoundTripper) {
	v.client.Transport = rt
}

func (v *VerifierClient) SetCircuitBreaker(cb *metrics.CircuitBreaker) {
	v.cb = cb
}
