package image

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

type MidjourneyProvider struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	logger     *slog.Logger
}

func NewMidjourneyProvider(apiKey, endpoint string, client *http.Client) *MidjourneyProvider {
	if client == nil {
		client = http.DefaultClient
	}
	return &MidjourneyProvider{
		apiKey:     apiKey,
		baseURL:    endpoint,
		httpClient: client,
		logger:     slog.Default(),
	}
}

func (m *MidjourneyProvider) SetLogger(logger *slog.Logger) {
	if logger != nil {
		m.logger = logger
	}
}

func (m *MidjourneyProvider) ProviderName() string {
	return "midjourney"
}

func (m *MidjourneyProvider) IsAvailable(_ context.Context) bool {
	return m.apiKey != "" && m.baseURL != ""
}

func (m *MidjourneyProvider) GenerateImage(ctx context.Context, req ImageRequest) (*ImageResult, error) {
	if m.apiKey == "" || m.baseURL == "" {
		return nil, fmt.Errorf("midjourney: API key or endpoint not configured")
	}

	fullPrompt := req.Prompt
	if req.Style != "" {
		fullPrompt = req.Prompt + ", " + req.Style
	}

	body := map[string]string{
		"prompt": fullPrompt,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("midjourney: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, m.baseURL+"/imagine", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("midjourney: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+m.apiKey)

	resp, err := m.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("midjourney: send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("midjourney: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("midjourney: API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ImageURL string `json:"image_url"`
		Prompt   string `json:"prompt"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("midjourney: decode response: %w", err)
	}

	return &ImageResult{
		URL:      result.ImageURL,
		Format:   "png",
		Provider: "midjourney",
		Prompt:   result.Prompt,
	}, nil
}
