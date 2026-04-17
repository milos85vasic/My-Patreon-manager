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

type OpenAICompatProvider struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
	logger     *slog.Logger
}

func NewOpenAICompatProvider(apiKey, baseURL, model string, client *http.Client) *OpenAICompatProvider {
	if client == nil {
		client = http.DefaultClient
	}
	return &OpenAICompatProvider{
		apiKey:     apiKey,
		baseURL:    baseURL,
		model:      model,
		httpClient: client,
		logger:     slog.Default(),
	}
}

func (o *OpenAICompatProvider) SetLogger(logger *slog.Logger) {
	if logger != nil {
		o.logger = logger
	}
}

func (o *OpenAICompatProvider) ProviderName() string {
	return "openai_compat"
}

func (o *OpenAICompatProvider) IsAvailable(_ context.Context) bool {
	return o.apiKey != "" && o.baseURL != ""
}

func (o *OpenAICompatProvider) GenerateImage(ctx context.Context, req ImageRequest) (*ImageResult, error) {
	if o.apiKey == "" || o.baseURL == "" {
		return nil, fmt.Errorf("openai_compat: API key or endpoint not configured")
	}

	fullPrompt := req.Prompt
	if req.Style != "" {
		fullPrompt = req.Prompt + ", " + req.Style
	}

	model := o.model
	if model == "" {
		model = "dall-e-3"
	}

	body := map[string]interface{}{
		"model":  model,
		"prompt": fullPrompt,
		"n":      1,
		"size":   req.Size,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("openai_compat: marshal request: %w", err)
	}

	url := o.baseURL + "/images/generations"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("openai_compat: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := o.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai_compat: send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openai_compat: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai_compat: API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Data []struct {
			URL           string `json:"url"`
			B64JSON       string `json:"b64_json"`
			RevisedPrompt string `json:"revised_prompt"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("openai_compat: decode response: %w", err)
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("openai_compat: no images in response")
	}

	img := result.Data[0]
	return &ImageResult{
		URL:      img.URL,
		Format:   "png",
		Provider: "openai_compat",
		Prompt:   img.RevisedPrompt,
	}, nil
}
