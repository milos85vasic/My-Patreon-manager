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
	p := NewDALLEProvider("test-key", nil)
	assert.Equal(t, "dalle", p.ProviderName())
}

func TestDALLEProvider_IsAvailable(t *testing.T) {
	p := NewDALLEProvider("test-key", nil)
	assert.True(t, p.IsAvailable(context.Background()))

	p2 := NewDALLEProvider("", nil)
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
					"url":            "https://example.com/image.png",
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

func TestDALLEProvider_GenerateImage_NoAPIKey(t *testing.T) {
	p := NewDALLEProvider("", nil)
	result, err := p.GenerateImage(context.Background(), ImageRequest{Prompt: "test"})
	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "API key not configured")
}

func TestDALLEProvider_GenerateImage_EmptyData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"data": []map[string]interface{}{},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewDALLEProvider("test-key", server.Client())
	p.baseURL = server.URL

	result, err := p.GenerateImage(context.Background(), ImageRequest{Prompt: "test"})
	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no images in response")
}
