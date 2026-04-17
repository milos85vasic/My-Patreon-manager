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

type DALLEProvider struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string
	model      string
	logger     *slog.Logger
}

func NewDALLEProvider(apiKey string, client *http.Client) *DALLEProvider {
	if client == nil {
		client = http.DefaultClient
	}
	return &DALLEProvider{
		apiKey:     apiKey,
		httpClient: client,
		baseURL:    "https://api.openai.com/v1",
		model:      "dall-e-3",
		logger:     slog.Default(),
	}
}

func (d *DALLEProvider) SetLogger(logger *slog.Logger) {
	if logger != nil {
		d.logger = logger
	}
}

func (d *DALLEProvider) ProviderName() string {
	return "dalle"
}

func (d *DALLEProvider) IsAvailable(_ context.Context) bool {
	return d.apiKey != ""
}

func (d *DALLEProvider) GenerateImage(ctx context.Context, req ImageRequest) (*ImageResult, error) {
	if d.apiKey == "" {
		return nil, fmt.Errorf("dalle: API key not configured")
	}

	fullPrompt := req.Prompt
	if req.Style != "" {
		fullPrompt = req.Prompt + ", " + req.Style
	}

	body := map[string]interface{}{
		"model":  d.model,
		"prompt": fullPrompt,
		"n":      1,
		"size":   req.Size,
	}
	if req.Quality == "hd" {
		body["quality"] = "hd"
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("dalle: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, d.baseURL+"/images/generations", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("dalle: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+d.apiKey)

	resp, err := d.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("dalle: send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("dalle: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dalle: API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Data []struct {
			URL           string `json:"url"`
			B64JSON       string `json:"b64_json"`
			RevisedPrompt string `json:"revised_prompt"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("dalle: decode response: %w", err)
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("dalle: no images in response")
	}

	img := result.Data[0]
	return &ImageResult{
		URL:      img.URL,
		Format:   "png",
		Provider: "dalle",
		Prompt:   img.RevisedPrompt,
	}, nil
}
