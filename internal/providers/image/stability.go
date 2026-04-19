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

type StabilityProvider struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string
	engine     string
	logger     *slog.Logger
}

// NewStabilityProvider constructs a Stability AI provider. A non-empty
// baseURL overrides the default Stability v2beta endpoint — useful for
// routing calls through a proxy or regional mirror. An empty baseURL
// falls back to the public Stability v2beta API.
func NewStabilityProvider(apiKey, baseURL string, client *http.Client) *StabilityProvider {
	if client == nil {
		client = http.DefaultClient
	}
	if baseURL == "" {
		baseURL = "https://api.stability.ai/v2beta"
	}
	return &StabilityProvider{
		apiKey:     apiKey,
		httpClient: client,
		baseURL:    baseURL,
		engine:     "stable-diffusion-xl-1.0",
		logger:     slog.Default(),
	}
}

func (s *StabilityProvider) SetLogger(logger *slog.Logger) {
	if logger != nil {
		s.logger = logger
	}
}

func (s *StabilityProvider) ProviderName() string {
	return "stability"
}

func (s *StabilityProvider) IsAvailable(_ context.Context) bool {
	return s.apiKey != ""
}

func (s *StabilityProvider) GenerateImage(ctx context.Context, req ImageRequest) (*ImageResult, error) {
	if s.apiKey == "" {
		return nil, fmt.Errorf("stability: API key not configured")
	}

	fullPrompt := req.Prompt
	if req.Style != "" {
		fullPrompt = req.Prompt + ", " + req.Style
	}

	body := map[string]interface{}{
		"prompt": fullPrompt,
		"output_format": func() string {
			if req.Format == "jpeg" {
				return "jpeg"
			}
			return "png"
		}(),
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("stability: marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/stable-image/generate/sdxl", s.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("stability: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+s.apiKey)
	httpReq.Header.Set("Accept", "image/"+req.Format)

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("stability: send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("stability: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("stability: API error %d: %s", resp.StatusCode, string(respBody))
	}

	format := req.Format
	if format == "" {
		format = "png"
	}
	return &ImageResult{
		Data:     respBody,
		Format:   format,
		Provider: "stability",
		Prompt:   fullPrompt,
	}, nil
}
