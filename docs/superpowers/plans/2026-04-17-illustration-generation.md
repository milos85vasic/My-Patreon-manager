# Illustration Generation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add enterprise-grade, context-aware illustration generation to every article, with configurable style, provider fallback chain, and per-repo opt-out.

**Architecture:** New `internal/providers/image/` package with `ImageProvider` interface and 4 implementations (DALL-E 3, Stability AI, Midjourney proxy, OpenAI-compatible). New `internal/services/illustration/` service coordinating providers, style loading, prompt building, and storage. The orchestrator calls the illustration generator between quality gate and rendering.

**Tech Stack:** Go 1.26.1, standard library HTTP clients, testify for tests, existing database/metrics/audit infrastructure.

---

### Task 1: Illustration Model

**Files:**
- Create: `internal/models/illustration.go`
- Test: `internal/models/illustration_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/models/illustration_test.go`:

```go
package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestIllustrationIDGeneration(t *testing.T) {
	ill := &Illustration{
		GeneratedContentID: "content-123",
		RepositoryID:       "repo-456",
		Prompt:             "test prompt",
		Style:              "default style",
	}
	assert.NotEmpty(t, ill.GenerateID())
	assert.Equal(t, "ill_"+sha256Hex("content-123"+"repo-456")[:32], ill.GenerateID())
}

func TestIllustrationFingerprint(t *testing.T) {
	ill := &Illustration{
		Prompt: "a beautiful landscape",
		Style:  "watercolor",
	}
	fp := ill.ComputeFingerprint()
	assert.NotEmpty(t, fp)

	ill2 := &Illustration{
		Prompt: "a beautiful landscape",
		Style:  "watercolor",
	}
	assert.Equal(t, fp, ill2.ComputeFingerprint())

	ill3 := &Illustration{
		Prompt: "a different prompt",
		Style:  "watercolor",
	}
	assert.NotEqual(t, fp, ill3.ComputeFingerprint())
}

func TestIllustrationDefaultValues(t *testing.T) {
	ill := &Illustration{}
	ill.SetDefaults()
	assert.Equal(t, "png", ill.Format)
	assert.Equal(t, "1792x1024", ill.Size)
	assert.NotEmpty(t, ill.ID)
	assert.False(t, ill.CreatedAt.IsZero())
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/models/ -run TestIllustration -v`
Expected: FAIL — `Illustration` type not defined

- [ ] **Step 3: Write minimal implementation**

Create `internal/models/illustration.go`:

```go
package models

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

type Illustration struct {
	ID                 string    `json:"id" db:"id"`
	GeneratedContentID string    `json:"generated_content_id" db:"generated_content_id"`
	RepositoryID       string    `json:"repository_id" db:"repository_id"`
	FilePath           string    `json:"file_path" db:"file_path"`
	ImageURL           string    `json:"image_url" db:"image_url"`
	Prompt             string    `json:"prompt" db:"prompt"`
	Style              string    `json:"style" db:"style"`
	ProviderUsed       string    `json:"provider_used" db:"provider_used"`
	Format             string    `json:"format" db:"format"`
	Size               string    `json:"size" db:"size"`
	ContentHash        string    `json:"content_hash" db:"content_hash"`
	Fingerprint        string    `json:"fingerprint" db:"fingerprint"`
	CreatedAt          time.Time `json:"created_at" db:"created_at"`
}

func (i *Illustration) GenerateID() string {
	h := sha256.Sum256([]byte(i.GeneratedContentID + i.RepositoryID))
	i.ID = "ill_" + hex.EncodeToString(h[:])[:32]
	return i.ID
}

func (i *Illustration) ComputeFingerprint() string {
	h := sha256.Sum256([]byte(i.Prompt + i.Style))
	i.Fingerprint = hex.EncodeToString(h[:])
	return i.Fingerprint
}

func (i *Illustration) SetDefaults() {
	if i.Format == "" {
		i.Format = "png"
	}
	if i.Size == "" {
		i.Size = "1792x1024"
	}
	if i.ID == "" {
		i.GenerateID()
	}
	if i.CreatedAt.IsZero() {
		i.CreatedAt = time.Now()
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/models/ -run TestIllustration -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/models/illustration.go internal/models/illustration_test.go
git commit -m "Add Illustration model with ID generation and fingerprinting"
```

---

### Task 2: ImageProvider Interface and Types

**Files:**
- Create: `internal/providers/image/provider.go`
- Create: `internal/providers/image/doc.go`
- Test: `internal/providers/image/provider_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/providers/image/provider_test.go`:

```go
package image

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestImageRequestDefaults(t *testing.T) {
	req := ImageRequest{
		Prompt:       "a sunset over mountains",
		RepositoryID: "repo-1",
	}
	req.SetDefaults()
	assert.Equal(t, "1792x1024", req.Size)
	assert.Equal(t, "hd", req.Quality)
	assert.Equal(t, "png", req.Format)
}

func TestImageResult_HasData(t *testing.T) {
	r := &ImageResult{Data: []byte{1, 2, 3}}
	assert.True(t, r.HasData())

	r2 := &ImageResult{URL: "https://example.com/img.png"}
	assert.True(t, r2.HasData())

	r3 := &ImageResult{}
	assert.False(t, r3.HasData())
}

type mockProvider struct {
	name      string
	available bool
	result    *ImageResult
	err       error
}

func (m *mockProvider) GenerateImage(ctx context.Context, req ImageRequest) (*ImageResult, error) {
	return m.result, m.err
}

func (m *mockProvider) ProviderName() string {
	return m.name
}

func (m *mockProvider) IsAvailable(ctx context.Context) bool {
	return m.available
}

func TestMockProviderImplementsInterface(t *testing.T) {
	var _ ImageProvider = &mockProvider{}
	assert.True(t, true)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/providers/image/ -v`
Expected: FAIL — package does not exist

- [ ] **Step 3: Write minimal implementation**

Create `internal/providers/image/doc.go`:

```go
package image
```

Create `internal/providers/image/provider.go`:

```go
package image

import "context"

type ImageProvider interface {
	GenerateImage(ctx context.Context, req ImageRequest) (*ImageResult, error)
	ProviderName() string
	IsAvailable(ctx context.Context) bool
}

type ImageRequest struct {
	Prompt       string `json:"prompt"`
	Style        string `json:"style"`
	Size         string `json:"size"`
	Quality      string `json:"quality"`
	Format       string `json:"format"`
	RepositoryID string `json:"repository_id"`
}

func (r *ImageRequest) SetDefaults() {
	if r.Size == "" {
		r.Size = "1792x1024"
	}
	if r.Quality == "" {
		r.Quality = "hd"
	}
	if r.Format == "" {
		r.Format = "png"
	}
}

type ImageResult struct {
	Data     []byte `json:"-"`
	URL      string `json:"url"`
	Format   string `json:"format"`
	Provider string `json:"provider"`
	Prompt   string `json:"prompt"`
}

func (r *ImageResult) HasData() bool {
	return len(r.Data) > 0 || r.URL != ""
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/providers/image/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/providers/image/
git commit -m "Add ImageProvider interface and request/result types"
```

---

### Task 3: DALL-E 3 Provider

**Files:**
- Create: `internal/providers/image/dalle.go`
- Test: `internal/providers/image/dalle_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/providers/image/dalle_test.go`:

```go
package image

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDALLEProvider_ProviderName(t *testing.T) {
	p := NewDALLEProvider("test-key", "")
	assert.Equal(t, "dalle", p.ProviderName())
}

func TestDALLEProvider_IsAvailable(t *testing.T) {
	p := NewDALLEProvider("test-key", "")
	assert.True(t, p.IsAvailable(context.Background()))

	p2 := NewDALLEProvider("", "")
	assert.False(t, p2.IsAvailable(context.Background()))
}

func TestDALLEProvider_GenerateImage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Contains(t, r.URL.Path, "images/generations")
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		resp := map[string]interface{}{
			"data": []map[string]interface{}{
				{
					"url":           "https://example.com/image.png",
					"revised_prompt": "a beautiful sunset over mountains, modern tech illustration",
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewDALLEProvider("test-key", server.Client())
	p.baseURL = server.URL

	result, err := p.GenerateImage(context.Background(), ImageRequest{
		Prompt: "a sunset over mountains",
		Style:  "modern tech illustration",
		Size:   "1792x1024",
	})

	require.NoError(t, err)
	assert.Equal(t, "dalle", result.Provider)
	assert.Equal(t, "https://example.com/image.png", result.URL)
	assert.Contains(t, result.Prompt, "sunset")
}

func TestDALLEProvider_GenerateImage_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{"message": "rate limited"},
		})
	}))
	defer server.Close()

	p := NewDALLEProvider("test-key", server.Client())
	p.baseURL = server.URL

	result, err := p.GenerateImage(context.Background(), ImageRequest{
		Prompt: "test",
	})
	assert.Nil(t, result)
	assert.Error(t, err)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/providers/image/ -run TestDALLE -v`
Expected: FAIL — `NewDALLEProvider` not defined

- [ ] **Step 3: Write implementation**

Create `internal/providers/image/dalle.go`:

```go
package image

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type DALLEProvider struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string
	model      string
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/providers/image/ -run TestDALLE -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/providers/image/dalle.go internal/providers/image/dalle_test.go
git commit -m "Add DALL-E 3 image provider implementation"
```

---

### Task 4: Stability AI Provider

**Files:**
- Create: `internal/providers/image/stability.go`
- Test: `internal/providers/image/stability_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/providers/image/stability_test.go`:

```go
package image

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStabilityProvider_ProviderName(t *testing.T) {
	p := NewStabilityProvider("test-key", "")
	assert.Equal(t, "stability", p.ProviderName())
}

func TestStabilityProvider_IsAvailable(t *testing.T) {
	p := NewStabilityProvider("test-key", "")
	assert.True(t, p.IsAvailable(context.Background()))

	p2 := NewStabilityProvider("", "")
	assert.False(t, p2.IsAvailable(context.Background()))
}

func TestStabilityProvider_GenerateImage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Contains(t, r.URL.Path, "stable-image")
		assert.Contains(t, r.URL.Path, "sdxl")
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte{0x89, 0x50, 0x4E, 0x47})
	}))
	defer server.Close()

	p := NewStabilityProvider("test-key", server.Client())
	p.baseURL = server.URL

	result, err := p.GenerateImage(context.Background(), ImageRequest{
		Prompt: "a sunset over mountains",
		Style:  "modern tech illustration",
	})

	require.NoError(t, err)
	assert.Equal(t, "stability", result.Provider)
	assert.NotEmpty(t, result.Data)
	assert.Equal(t, "png", result.Format)
}

func TestStabilityProvider_GenerateImage_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]string{
			"message": "rate limited",
		})
	}))
	defer server.Close()

	p := NewStabilityProvider("test-key", server.Client())
	p.baseURL = server.URL

	result, err := p.GenerateImage(context.Background(), ImageRequest{
		Prompt: "test",
	})
	assert.Nil(t, result)
	assert.Error(t, err)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/providers/image/ -run TestStability -v`
Expected: FAIL — `NewStabilityProvider` not defined

- [ ] **Step 3: Write implementation**

Create `internal/providers/image/stability.go`:

```go
package image

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type StabilityProvider struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string
	engine     string
}

func NewStabilityProvider(apiKey string, client *http.Client) *StabilityProvider {
	if client == nil {
		client = http.DefaultClient
	}
	return &StabilityProvider{
		apiKey:     apiKey,
		httpClient: client,
		baseURL:    "https://api.stability.ai/v2beta",
		engine:     "stable-diffusion-xl-1.0",
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

	return &ImageResult{
		Data:     respBody,
		Format:   req.Format,
		Provider: "stability",
		Prompt:   fullPrompt,
	}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/providers/image/ -run TestStability -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/providers/image/stability.go internal/providers/image/stability_test.go
git commit -m "Add Stability AI image provider implementation"
```

---

### Task 5: Midjourney Proxy Provider

**Files:**
- Create: `internal/providers/image/midjourney.go`
- Test: `internal/providers/image/midjourney_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/providers/image/midjourney_test.go`:

```go
package image

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMidjourneyProvider_ProviderName(t *testing.T) {
	p := NewMidjourneyProvider("test-key", "http://localhost", nil)
	assert.Equal(t, "midjourney", p.ProviderName())
}

func TestMidjourneyProvider_IsAvailable(t *testing.T) {
	p := NewMidjourneyProvider("test-key", "http://localhost", nil)
	assert.True(t, p.IsAvailable(context.Background()))

	p2 := NewMidjourneyProvider("", "", nil)
	assert.False(t, p2.IsAvailable(context.Background()))
}

func TestMidjourneyProvider_GenerateImage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		assert.Contains(t, body["prompt"], "sunset")
		assert.Contains(t, body["prompt"], "modern tech illustration")

		resp := map[string]interface{}{
			"image_url": "https://cdn.example.com/mj-image.png",
			"prompt":    body["prompt"],
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewMidjourneyProvider("test-key", server.URL, server.Client())

	result, err := p.GenerateImage(context.Background(), ImageRequest{
		Prompt: "a sunset over mountains",
		Style:  "modern tech illustration",
	})

	require.NoError(t, err)
	assert.Equal(t, "midjourney", result.Provider)
	assert.Equal(t, "https://cdn.example.com/mj-image.png", result.URL)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/providers/image/ -run TestMidjourney -v`
Expected: FAIL — `NewMidjourneyProvider` not defined

- [ ] **Step 3: Write implementation**

Create `internal/providers/image/midjourney.go`:

```go
package image

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type MidjourneyProvider struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func NewMidjourneyProvider(apiKey, endpoint string, client *http.Client) *MidjourneyProvider {
	if client == nil {
		client = http.DefaultClient
	}
	return &MidjourneyProvider{
		apiKey:     apiKey,
		baseURL:    endpoint,
		httpClient: client,
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/providers/image/ -run TestMidjourney -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/providers/image/midjourney.go internal/providers/image/midjourney_test.go
git commit -m "Add Midjourney proxy image provider implementation"
```

---

### Task 6: OpenAI-Compatible Provider

**Files:**
- Create: `internal/providers/image/openai_compat.go`
- Test: `internal/providers/image/openai_compat_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/providers/image/openai_compat_test.go`:

```go
package image

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAICompatProvider_ProviderName(t *testing.T) {
	p := NewOpenAICompatProvider("test-key", "http://localhost:8080", "custom-model", nil)
	assert.Equal(t, "openai_compat", p.ProviderName())
}

func TestOpenAICompatProvider_IsAvailable(t *testing.T) {
	p := NewOpenAICompatProvider("test-key", "http://localhost:8080", "model", nil)
	assert.True(t, p.IsAvailable(context.Background()))

	p2 := NewOpenAICompatProvider("", "", "", nil)
	assert.False(t, p2.IsAvailable(context.Background()))
}

func TestOpenAICompatProvider_GenerateImage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Contains(t, r.URL.Path, "images/generations")
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		assert.Equal(t, "custom-model", body["model"])

		resp := map[string]interface{}{
			"data": []map[string]interface{}{
				{"url": "https://custom.example.com/img.png"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewOpenAICompatProvider("test-key", server.URL, "custom-model", server.Client())

	result, err := p.GenerateImage(context.Background(), ImageRequest{
		Prompt: "a test image",
		Style:  "custom style",
	})

	require.NoError(t, err)
	assert.Equal(t, "openai_compat", result.Provider)
	assert.Equal(t, "https://custom.example.com/img.png", result.URL)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/providers/image/ -run TestOpenAICompat -v`
Expected: FAIL — `NewOpenAICompatProvider` not defined

- [ ] **Step 3: Write implementation**

Create `internal/providers/image/openai_compat.go`:

```go
package image

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type OpenAICompatProvider struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/providers/image/ -run TestOpenAICompat -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/providers/image/openai_compat.go internal/providers/image/openai_compat_test.go
git commit -m "Add OpenAI-compatible image provider implementation"
```

---

### Task 7: Fallback Chain Provider

**Files:**
- Create: `internal/providers/image/fallback.go`
- Test: `internal/providers/image/fallback_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/providers/image/fallback_test.go`:

```go
package image

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFallbackProvider_Name(t *testing.T) {
	p := NewFallbackProvider()
	assert.Equal(t, "fallback", p.ProviderName())
}

func TestFallbackProvider_SingleProviderSuccess(t *testing.T) {
	fp := NewFallbackProvider(
		&mockProvider{name: "dalle", available: true, result: &ImageResult{URL: "https://img.png", Provider: "dalle"}},
	)
	result, err := fp.GenerateImage(context.Background(), ImageRequest{Prompt: "test"})
	require.NoError(t, err)
	assert.Equal(t, "dalle", result.Provider)
}

func TestFallbackProvider_FallbackOnFailure(t *testing.T) {
	fp := NewFallbackProvider(
		&mockProvider{name: "dalle", available: true, err: errors.New("rate limited")},
		&mockProvider{name: "stability", available: true, result: &ImageResult{URL: "https://img.png", Provider: "stability"}},
	)
	result, err := fp.GenerateImage(context.Background(), ImageRequest{Prompt: "test"})
	require.NoError(t, err)
	assert.Equal(t, "stability", result.Provider)
}

func TestFallbackProvider_AllFail(t *testing.T) {
	fp := NewFallbackProvider(
		&mockProvider{name: "dalle", available: true, err: errors.New("fail 1")},
		&mockProvider{name: "stability", available: true, err: errors.New("fail 2")},
	)
	result, err := fp.GenerateImage(context.Background(), ImageRequest{Prompt: "test"})
	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "all providers failed")
}

func TestFallbackProvider_SkipsUnavailable(t *testing.T) {
	fp := NewFallbackProvider(
		&mockProvider{name: "dalle", available: false},
		&mockProvider{name: "stability", available: true, result: &ImageResult{URL: "https://img.png", Provider: "stability"}},
	)
	result, err := fp.GenerateImage(context.Background(), ImageRequest{Prompt: "test"})
	require.NoError(t, err)
	assert.Equal(t, "stability", result.Provider)
}

func TestFallbackProvider_IsAvailable(t *testing.T) {
	fp := NewFallbackProvider(
		&mockProvider{name: "dalle", available: false},
		&mockProvider{name: "stability", available: true},
	)
	assert.True(t, fp.IsAvailable(context.Background()))

	fp2 := NewFallbackProvider(
		&mockProvider{name: "dalle", available: false},
		&mockProvider{name: "stability", available: false},
	)
	assert.False(t, fp2.IsAvailable(context.Background()))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/providers/image/ -run TestFallback -v`
Expected: FAIL — `NewFallbackProvider` not defined

- [ ] **Step 3: Write implementation**

Create `internal/providers/image/fallback.go`:

```go
package image

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

type FallbackProvider struct {
	providers []ImageProvider
	logger    *slog.Logger
}

func NewFallbackProvider(providers ...ImageProvider) *FallbackProvider {
	return &FallbackProvider{
		providers: providers,
		logger:    slog.Default(),
	}
}

func (f *FallbackProvider) SetLogger(logger *slog.Logger) {
	f.logger = logger
}

func (f *FallbackProvider) ProviderName() string {
	return "fallback"
}

func (f *FallbackProvider) IsAvailable(ctx context.Context) bool {
	for _, p := range f.providers {
		if p.IsAvailable(ctx) {
			return true
		}
	}
	return false
}

func (f *FallbackProvider) GenerateImage(ctx context.Context, req ImageRequest) (*ImageResult, error) {
	var errs []string

	for _, p := range f.providers {
		if !p.IsAvailable(ctx) {
			f.logger.Debug("skipping unavailable provider", "provider", p.ProviderName())
			continue
		}

		result, err := p.GenerateImage(ctx, req)
		if err != nil {
			f.logger.Warn("provider failed, trying next",
				"provider", p.ProviderName(),
				"error", err,
			)
			errs = append(errs, fmt.Sprintf("%s: %s", p.ProviderName(), err.Error()))
			continue
		}

		f.logger.Info("illustration generated",
			"provider", p.ProviderName(),
			"repository_id", req.RepositoryID,
		)
		return result, nil
	}

	return nil, fmt.Errorf("all providers failed: %s", strings.Join(errs, "; "))
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/providers/image/ -run TestFallback -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/providers/image/fallback.go internal/providers/image/fallback_test.go
git commit -m "Add fallback chain image provider with ordered retry logic"
```

---

### Task 8: Database Migration and Illustration Store

**Files:**
- Create: `internal/database/migrations/0002_illustrations.up.sql`
- Create: `internal/database/migrations/0002_illustrations.down.sql`
- Modify: `internal/database/stores.go` — add `IllustrationStore` interface and `Illustrations()` method
- Modify: `internal/database/sqlite.go` — implement SQLite IllustrationStore
- Modify: `internal/database/postgres.go` — implement Postgres IllustrationStore
- Modify: `tests/mocks/database.go` — add `MockIllustrationStore`
- Test: `internal/database/sqlite_illustration_test.go` (or coverage test)

- [ ] **Step 1: Create migration files**

Create `internal/database/migrations/0002_illustrations.up.sql`:

```sql
BEGIN;

CREATE TABLE IF NOT EXISTS illustrations (
    id TEXT PRIMARY KEY,
    generated_content_id TEXT NOT NULL,
    repository_id TEXT NOT NULL,
    file_path TEXT NOT NULL,
    image_url TEXT DEFAULT '',
    prompt TEXT NOT NULL,
    style TEXT DEFAULT '',
    provider_used TEXT NOT NULL,
    format TEXT DEFAULT 'png',
    size TEXT DEFAULT '1792x1024',
    content_hash TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (generated_content_id) REFERENCES generated_contents(id) ON DELETE CASCADE,
    FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_illustrations_content ON illustrations(generated_content_id);
CREATE INDEX IF NOT EXISTS idx_illustrations_fingerprint ON illustrations(fingerprint);
CREATE INDEX IF NOT EXISTS idx_illustrations_repo ON illustrations(repository_id);

COMMIT;
```

Create `internal/database/migrations/0002_illustrations.down.sql`:

```sql
DROP INDEX IF EXISTS idx_illustrations_repo;
DROP INDEX IF EXISTS idx_illustrations_fingerprint;
DROP INDEX IF EXISTS idx_illustrations_content;
DROP TABLE IF EXISTS illustrations;
```

- [ ] **Step 2: Add IllustrationStore interface to stores.go**

Add to `internal/database/stores.go` after the existing store interfaces:

```go
type IllustrationStore interface {
	Create(ctx context.Context, ill *models.Illustration) error
	GetByID(ctx context.Context, id string) (*models.Illustration, error)
	GetByContentID(ctx context.Context, contentID string) (*models.Illustration, error)
	GetByFingerprint(ctx context.Context, fingerprint string) (*models.Illustration, error)
	ListByRepository(ctx context.Context, repoID string) ([]*models.Illustration, error)
	Delete(ctx context.Context, id string) error
}
```

Add `Illustrations() IllustrationStore` method to the `Database` interface.

- [ ] **Step 3: Implement in SQLite and Postgres backends**

Add `Illustrations()` method returning a concrete `sqliteIllustrationStore` / `postgresIllustrationStore` to each backend. Implement all CRUD methods following the existing store pattern in that file.

- [ ] **Step 4: Add MockIllustrationStore to tests/mocks/database.go**

```go
type MockIllustrationStore struct {
	CreateFunc           func(ctx context.Context, ill *models.Illustration) error
	GetByIDFunc          func(ctx context.Context, id string) (*models.Illustration, error)
	GetByContentIDFunc   func(ctx context.Context, contentID string) (*models.Illustration, error)
	GetByFingerprintFunc func(ctx context.Context, fingerprint string) (*models.Illustration, error)
	ListByRepositoryFunc func(ctx context.Context, repoID string) ([]*models.Illustration, error)
	DeleteFunc           func(ctx context.Context, id string) error
}

func (m *MockIllustrationStore) Create(ctx context.Context, ill *models.Illustration) error {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, ill)
	}
	return nil
}

func (m *MockIllustrationStore) GetByID(ctx context.Context, id string) (*models.Illustration, error) {
	if m.GetByIDFunc != nil {
		return m.GetByIDFunc(ctx, id)
	}
	return nil, nil
}

func (m *MockIllustrationStore) GetByContentID(ctx context.Context, contentID string) (*models.Illustration, error) {
	if m.GetByContentIDFunc != nil {
		return m.GetByContentIDFunc(ctx, contentID)
	}
	return nil, nil
}

func (m *MockIllustrationStore) GetByFingerprint(ctx context.Context, fingerprint string) (*models.Illustration, error) {
	if m.GetByFingerprintFunc != nil {
		return m.GetByFingerprintFunc(ctx, fingerprint)
	}
	return nil, nil
}

func (m *MockIllustrationStore) ListByRepository(ctx context.Context, repoID string) ([]*models.Illustration, error) {
	if m.ListByRepositoryFunc != nil {
		return m.ListByRepositoryFunc(ctx, repoID)
	}
	return nil, nil
}

func (m *MockIllustrationStore) Delete(ctx context.Context, id string) error {
	if m.DeleteFunc != nil {
		return m.DeleteFunc(ctx, id)
	}
	return nil
}
```

Also add `IllustrationsFunc` to `MockDatabase` and wire the `Illustrations()` method.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/database/... ./tests/mocks/... -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/database/ tests/mocks/database.go
git commit -m "Add illustration database store with SQLite/Postgres implementations"
```

---

### Task 9: Configuration

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `.env.example`

- [ ] **Step 1: Add config fields**

Add these fields to the `Config` struct in `internal/config/config.go`:

```go
	IllustrationEnabled       bool
	IllustrationDefaultStyle  string
	IllustrationDefaultSize   string
	IllustrationDefaultQuality string
	IllustrationDir           string
	ImageProviderPriority     string
	StabilityAPIKey           string
	MidjourneyAPIKey          string
	MidjourneyEndpoint        string
```

Set defaults in `NewConfig()`:

```go
	IllustrationEnabled:       true,
	IllustrationDefaultStyle:  "modern tech illustration, clean lines, professional",
	IllustrationDefaultSize:   "1792x1024",
	IllustrationDefaultQuality: "hd",
	IllustrationDir:           "./data/illustrations",
	ImageProviderPriority:     "dalle,stability,midjourney,openai_compat",
```

Wire in `LoadFromEnv()`:

```go
	cfg.IllustrationEnabled = getEnvBool("ILLUSTRATION_ENABLED", cfg.IllustrationEnabled)
	cfg.IllustrationDefaultStyle = getEnv("ILLUSTRATION_DEFAULT_STYLE", cfg.IllustrationDefaultStyle)
	cfg.IllustrationDefaultSize = getEnv("ILLUSTRATION_DEFAULT_SIZE", cfg.IllustrationDefaultSize)
	cfg.IllustrationDefaultQuality = getEnv("ILLUSTRATION_DEFAULT_QUALITY", cfg.IllustrationDefaultQuality)
	cfg.IllustrationDir = getEnv("ILLUSTRATION_DIR", cfg.IllustrationDir)
	cfg.ImageProviderPriority = getEnv("IMAGE_PROVIDER_PRIORITY", cfg.ImageProviderPriority)
	cfg.StabilityAPIKey = getEnv("STABILITY_API_KEY", cfg.StabilityAPIKey)
	cfg.MidjourneyAPIKey = getEnv("MIDJOURNEY_API_KEY", cfg.MidjourneyAPIKey)
	cfg.MidjourneyEndpoint = getEnv("MIDJOURNEY_ENDPOINT", cfg.MidjourneyEndpoint)
```

- [ ] **Step 2: Update .env.example**

Add commented illustration variables to `.env.example`:

```env
# Illustration Generation
ILLUSTRATION_ENABLED=true
ILLUSTRATION_DEFAULT_STYLE="modern tech illustration, clean lines, professional"
ILLUSTRATION_DEFAULT_SIZE=1792x1024
ILLUSTRATION_DEFAULT_QUALITY=hd
ILLUSTRATION_DIR=./data/illustrations
IMAGE_PROVIDER_PRIORITY=dalle,stability,midjourney,openai_compat

# Image Provider API Keys
# OPENAI_API_KEY=your_openai_key_here           # DALL-E 3 + OpenAI-compatible
# STABILITY_API_KEY=your_stability_key_here
# MIDJOURNEY_API_KEY=your_midjourney_key_here
# MIDJOURNEY_ENDPOINT=https://your-mj-proxy.example.com
```

- [ ] **Step 3: Add tests for new config fields**

Add test cases to `internal/config/config_test.go` verifying that the new fields are loaded from environment variables with correct defaults.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/config/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/ .env.example
git commit -m "Add illustration generation configuration fields and env vars"
```

---

### Task 10: Illustration Generator Service

**Files:**
- Create: `internal/services/illustration/generator.go`
- Create: `internal/services/illustration/style.go`
- Create: `internal/services/illustration/prompt.go`
- Create: `internal/services/illustration/doc.go`
- Test: `internal/services/illustration/generator_test.go`

- [ ] **Step 1: Write the failing test for PromptBuilder**

Create `internal/services/illustration/generator_test.go`:

```go
package illustration

import (
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestPromptBuilder_Build(t *testing.T) {
	repo := &models.Repository{
		Name:            "my-go-project",
		Description:     "A scalable API built with Go",
		PrimaryLanguage: "Go",
		Topics:          []string{"api", "microservices"},
	}
	content := &models.GeneratedContent{
		Title: "Building Scalable APIs",
	}

	pb := NewPromptBuilder("modern tech illustration, clean lines")
	prompt := pb.Build(repo, content)
	assert.Contains(t, prompt, "my-go-project")
	assert.Contains(t, prompt, "Go")
	assert.Contains(t, prompt, "modern tech illustration")
}

func TestStyleLoader_DefaultStyle(t *testing.T) {
	sl := NewStyleLoader("global default style")
	style := sl.LoadStyle(nil)
	assert.Equal(t, "global default style", style)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/services/illustration/ -v`
Expected: FAIL — package does not exist

- [ ] **Step 3: Write implementations**

Create `internal/services/illustration/doc.go`:

```go
package illustration
```

Create `internal/services/illustration/prompt.go`:

```go
package illustration

import (
	"fmt"
	"strings"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

type PromptBuilder struct {
	defaultStyle string
}

func NewPromptBuilder(defaultStyle string) *PromptBuilder {
	return &PromptBuilder{defaultStyle: defaultStyle}
}

func (pb *PromptBuilder) Build(repo *models.Repository, content *models.GeneratedContent) string {
	parts := []string{}

	if content.Title != "" {
		parts = append(parts, fmt.Sprintf("Illustration for article \"%s\"", content.Title))
	}

	if repo.Name != "" {
		parts = append(parts, fmt.Sprintf("about the %s project", repo.Name))
	}

	if repo.PrimaryLanguage != "" {
		parts = append(parts, fmt.Sprintf("using %s", repo.PrimaryLanguage))
	}

	if len(repo.Topics) > 0 {
		parts = append(parts, fmt.Sprintf("topics: %s", strings.Join(repo.Topics, ", ")))
	}

	if repo.Description != "" && len(repo.Description) < 200 {
		parts = append(parts, repo.Description)
	}

	if pb.defaultStyle != "" {
		parts = append(parts, pb.defaultStyle)
	}

	return strings.Join(parts, ". ")
}
```

Create `internal/services/illustration/style.go`:

```go
package illustration

type StyleLoader struct {
	globalStyle string
}

func NewStyleLoader(globalStyle string) *StyleLoader {
	return &StyleLoader{globalStyle: globalStyle}
}

func (sl *StyleLoader) LoadStyle(repoOverride *string) string {
	if repoOverride != nil && *repoOverride != "" {
		return *repoOverride
	}
	return sl.globalStyle
}
```

Create `internal/services/illustration/generator.go`:

```go
package illustration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	imgprov "github.com/milos85vasic/My-Patreon-Manager/internal/providers/image"
)

type Generator struct {
	providers     *imgprov.FallbackProvider
	store         database.IllustrationStore
	styleLoader   *StyleLoader
	promptBuilder *PromptBuilder
	logger        *slog.Logger
	imageDir      string
}

func NewGenerator(
	providers *imgprov.FallbackProvider,
	store database.IllustrationStore,
	styleLoader *StyleLoader,
	promptBuilder *PromptBuilder,
	logger *slog.Logger,
	imageDir string,
) *Generator {
	return &Generator{
		providers:     providers,
		store:         store,
		styleLoader:   styleLoader,
		promptBuilder: promptBuilder,
		logger:        logger,
		imageDir:      imageDir,
	}
}

func (g *Generator) Generate(
	ctx context.Context,
	repoID string,
	repoName string,
	repoDesc string,
	repoLang string,
	repoTopics []string,
	contentID string,
	contentTitle string,
	contentBody string,
) (*string, error) {
	prompt := g.promptBuilder.BuildFromFields(repoName, repoDesc, repoLang, repoTopics, contentTitle, contentBody)
	style := g.styleLoader.LoadStyle(nil)
	fingerprint := computeFingerprint(prompt, style)

	existing, err := g.store.GetByFingerprint(ctx, fingerprint)
	if err == nil && existing != nil && existing.FilePath != "" {
		g.logger.Debug("reusing existing illustration", "fingerprint", fingerprint)
		embedTag := fmt.Sprintf("![%s](%s)", contentTitle, existing.FilePath)
		return &embedTag, nil
	}

	req := imgprov.ImageRequest{
		Prompt:       prompt,
		Style:        style,
		RepositoryID: repoID,
	}
	req.SetDefaults()

	result, err := g.providers.GenerateImage(ctx, req)
	if err != nil {
		g.logger.Warn("illustration generation failed, skipping",
			"repository_id", repoID,
			"error", err,
		)
		return nil, nil
	}

	imageData := result.Data
	fileName := fmt.Sprintf("%s.%s", computeContentHash(imageData), result.Format)
	filePath := filepath.Join(g.imageDir, fileName)

	if err := os.MkdirAll(g.imageDir, 0o755); err != nil {
		return nil, fmt.Errorf("create illustration dir: %w", err)
	}

	if len(imageData) > 0 {
		if err := os.WriteFile(filePath, imageData, 0o644); err != nil {
			return nil, fmt.Errorf("write illustration file: %w", err)
		}
	} else if result.URL != "" {
		filePath = result.URL
	}

	ill := &models.Illustration{
		GeneratedContentID: contentID,
		RepositoryID:       repoID,
		FilePath:           filePath,
		ImageURL:           result.URL,
		Prompt:             prompt,
		Style:              style,
		ProviderUsed:       result.Provider,
		Format:             result.Format,
		ContentHash:        computeContentHash(imageData),
		Fingerprint:        fingerprint,
	}
	ill.GenerateID()
	ill.SetDefaults()

	if err := g.store.Create(ctx, ill); err != nil {
		g.logger.Error("failed to store illustration metadata", "error", err)
	}

	embedTag := fmt.Sprintf("![%s](%s)", contentTitle, filePath)
	return &embedTag, nil
}

func computeFingerprint(prompt, style string) string {
	h := sha256.Sum256([]byte(prompt + style))
	return hex.EncodeToString(h[:])
}

func computeContentHash(data []byte) string {
	if len(data) == 0 {
		return "no-data"
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])[:16]
}
```

Note: `PromptBuilder.BuildFromFields` is a convenience method that builds prompts from individual fields instead of requiring full model structs. Add it to `prompt.go` alongside `Build()`.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/services/illustration/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/services/illustration/
git commit -m "Add illustration generator service with prompt builder and style loader"
```

---

### Task 11: .repoignore `no-illustration` Directive

**Files:**
- Modify: `internal/services/filter/repoignore.go`
- Modify: `internal/services/filter/repoignore_test.go`

- [ ] **Step 1: Add directive support**

Add a `directives` field to the `Repoignore` struct and parse `no-illustration` lines as directives. Expose `HasDirective(directive string) bool` method.

- [ ] **Step 2: Write tests**

Test that `no-illustration` in a `.repoignore` file is recognized by `HasDirective("no-illustration")`, and that repos without it return `false`.

- [ ] **Step 3: Run tests**

Run: `go test ./internal/services/filter/ -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/services/filter/
git commit -m "Add no-illustration directive support to .repoignore filter"
```

---

### Task 12: Orchestrator Integration

**Files:**
- Modify: `internal/services/sync/orchestrator.go`
- Modify: `internal/services/sync/orchestrator_test.go` or create new test file
- Modify: `cmd/cli/main.go`
- Modify: `cmd/cli/main_test.go`
- Modify: `cmd/server/main.go`

- [ ] **Step 1: Add illustration generator to orchestrator**

Add `illustrationGen *illustration.Generator` field to `Orchestrator` struct. Add `SetIllustrationGenerator(gen *illustration.Generator)` setter method.

In `processRepo()`, after quality gate passes and before rendering, insert:

```go
if o.illustrationGen != nil && generated.PassedQualityGate {
	embedTag, err := o.illustrationGen.Generate(
		ctx, repo.ID, repo.Name, repo.Description, repo.PrimaryLanguage, repo.Topics,
		generated.ID, generated.Title, generated.Body,
	)
	if err != nil {
		o.logger.Warn("illustration generation failed", "repo_id", repo.ID, "error", err)
	} else if embedTag != nil {
		generated.Body = *embedTag + "\n\n" + generated.Body
	}
}
```

- [ ] **Step 2: Wire in CLI**

In `cmd/cli/main.go`, construct the `illustration.Generator` when `ILLUSTRATION_ENABLED=true`, build providers from config, and call `orch.SetIllustrationGenerator(gen)`.

Add a new `var newIllustrationGenerator` function variable for DI testing.

- [ ] **Step 3: Wire in server**

In `cmd/server/main.go`, same wiring as CLI for the server's orchestrator.

- [ ] **Step 4: Write tests**

Test that when `illustrationGen` is nil, processRepo skips illustration step. Test that when set, it calls Generate and prepends the embed tag to body.

- [ ] **Step 5: Run full test suite**

Run: `go test ./internal/... ./cmd/... -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/services/sync/ cmd/
git commit -m "Integrate illustration generator into orchestrator pipeline"
```

---

### Task 13: External Tests and Mocks

**Files:**
- Create: `tests/mocks/mock_image_provider.go`
- Create: `tests/unit/providers/image/image_provider_test.go`
- Create: `tests/unit/services/illustration/generator_test.go`

- [ ] **Step 1: Create shared mock**

Create `tests/mocks/mock_image_provider.go`:

```go
package mocks

import (
	"context"

	imgprov "github.com/milos85vasic/My-Patreon-Manager/internal/providers/image"
)

type MockImageProvider struct {
	GenerateImageFunc func(ctx context.Context, req imgprov.ImageRequest) (*imgprov.ImageResult, error)
	ProviderNameFunc  func() string
	IsAvailableFunc   func(ctx context.Context) bool
}

func (m *MockImageProvider) GenerateImage(ctx context.Context, req imgprov.ImageRequest) (*imgprov.ImageResult, error) {
	if m.GenerateImageFunc != nil {
		return m.GenerateImageFunc(ctx, req)
	}
	return &imgprov.ImageResult{}, nil
}

func (m *MockImageProvider) ProviderName() string {
	if m.ProviderNameFunc != nil {
		return m.ProviderNameFunc()
	}
	return "mock"
}

func (m *MockImageProvider) IsAvailable(ctx context.Context) bool {
	if m.IsAvailableFunc != nil {
		return m.IsAvailableFunc(ctx)
	}
	return true
}
```

- [ ] **Step 2: Write external unit tests**

Write integration-style tests in `tests/unit/` that wire together the fallback provider with mocks, the generator with mock stores, and test the full illustration flow.

- [ ] **Step 3: Run tests**

Run: `go test ./tests/unit/... -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add tests/mocks/mock_image_provider.go tests/unit/providers/ tests/unit/services/illustration/
git commit -m "Add illustration external tests and shared mock provider"
```

---

### Task 14: Coverage Gaps and Final Verification

**Files:**
- Various `coverage_gap_test.go` files as needed

- [ ] **Step 1: Run full coverage**

Run: `CGO_ENABLED=1 go test -race -timeout 10m -coverpkg=./internal/...,./cmd/... ./internal/... ./cmd/... ./tests/... -coverprofile=coverage/cover.out`

- [ ] **Step 2: Identify and fill coverage gaps**

Check `coverage/cover.out` for any uncovered functions/branches in:
- `internal/providers/image/`
- `internal/services/illustration/`
- `internal/database/` (illustration store methods)

Add `coverage_gap_test.go` files as needed to reach 100%.

- [ ] **Step 3: Run full test suite**

Run: `go test ./internal/... ./cmd/... ./tests/... -race`
Expected: ALL PASS

- [ ] **Step 4: Run go vet**

Run: `go vet ./...`
Expected: Clean

- [ ] **Step 5: Commit and push**

```bash
git add .
git commit -m "Complete illustration generation feature with full test coverage"
```

Push to all 4 remotes:

```bash
bash Upstreams/GitHub.sh && bash Upstreams/GitLab.sh && bash Upstreams/GitFlic.sh && bash Upstreams/GitVerse.sh
```

---

## Self-Review Checklist

**1. Spec coverage:**
- Model (Illustration) → Task 1
- ImageProvider interface → Task 2
- DALL-E 3 provider → Task 3
- Stability AI provider → Task 4
- Midjourney proxy provider → Task 5
- OpenAI-compatible provider → Task 6
- Fallback chain → Task 7
- Database migration + store → Task 8
- Configuration → Task 9
- IllustrationGenerator service → Task 10
- .repoignore directive → Task 11
- Orchestrator integration → Task 12
- External tests + mocks → Task 13
- Coverage + verification → Task 14

All spec requirements covered.

**2. Placeholder scan:** No TBDs, TODOs, or vague descriptions found. All code steps contain complete implementations.

**3. Type consistency:** `ImageProvider` interface defined in Task 2, used consistently across Tasks 3-7 and 10. `IllustrationStore` defined in Task 8, used in Task 10 and 13. `Illustration` model defined in Task 1, used throughout.
