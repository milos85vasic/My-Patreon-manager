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

	p3 := NewOpenAICompatProvider("key", "", "", nil)
	assert.False(t, p3.IsAvailable(context.Background()))
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

func TestOpenAICompatProvider_GenerateImage_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "fail"})
	}))
	defer server.Close()

	p := NewOpenAICompatProvider("test-key", server.URL, "model", server.Client())
	result, err := p.GenerateImage(context.Background(), ImageRequest{Prompt: "test"})
	assert.Nil(t, result)
	assert.Error(t, err)
}

func TestOpenAICompatProvider_GenerateImage_NotConfigured(t *testing.T) {
	p := NewOpenAICompatProvider("", "", "", nil)
	result, err := p.GenerateImage(context.Background(), ImageRequest{Prompt: "test"})
	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "API key or endpoint not configured")
}

func TestOpenAICompatProvider_DefaultModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		assert.Equal(t, "dall-e-3", body["model"])

		resp := map[string]interface{}{
			"data": []map[string]interface{}{
				{"url": "https://example.com/img.png"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewOpenAICompatProvider("test-key", server.URL, "", server.Client())
	result, err := p.GenerateImage(context.Background(), ImageRequest{Prompt: "test"})
	require.NoError(t, err)
	assert.NotNil(t, result)
}
