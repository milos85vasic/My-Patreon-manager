package image

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDALLEProvider_ProviderName(t *testing.T) {
	p := NewDALLEProvider("test-key", "", nil)
	assert.Equal(t, "dalle", p.ProviderName())
}

func TestDALLEProvider_IsAvailable(t *testing.T) {
	p := NewDALLEProvider("test-key", "", nil)
	assert.True(t, p.IsAvailable(context.Background()))

	p2 := NewDALLEProvider("", "", nil)
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

	p := NewDALLEProvider("test-key", server.URL, server.Client())

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

	p := NewDALLEProvider("test-key", server.URL, server.Client())

	result, err := p.GenerateImage(context.Background(), ImageRequest{
		Prompt: "test",
	})
	assert.Nil(t, result)
	assert.Error(t, err)
}

func TestDALLEProvider_GenerateImage_NoAPIKey(t *testing.T) {
	p := NewDALLEProvider("", "", nil)
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

	p := NewDALLEProvider("test-key", server.URL, server.Client())

	result, err := p.GenerateImage(context.Background(), ImageRequest{Prompt: "test"})
	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no images in response")
}

// TestDALLEProvider_CustomBaseURL confirms that a custom base URL passed
// to the constructor is honored for the request path.
func TestDALLEProvider_CustomBaseURL(t *testing.T) {
	var gotURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.Path
		resp := map[string]interface{}{
			"data": []map[string]interface{}{
				{"url": "https://example.com/image.png", "revised_prompt": "x"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewDALLEProvider("test-key", server.URL+"/custom/v2", server.Client())
	_, err := p.GenerateImage(context.Background(), ImageRequest{
		Prompt: "p", Size: "1792x1024",
	})
	require.NoError(t, err)
	assert.Equal(t, "/custom/v2/images/generations", gotURL)
	assert.Equal(t, server.URL+"/custom/v2", p.baseURL)
}

// TestDALLEProvider_EmptyBaseURLFallsBack confirms the public OpenAI
// endpoint default is applied when no base URL is supplied.
func TestDALLEProvider_EmptyBaseURLFallsBack(t *testing.T) {
	p := NewDALLEProvider("test-key", "", nil)
	assert.Equal(t, "https://api.openai.com/v1", p.baseURL)
}

func TestDALLEProvider_SetLogger(t *testing.T) {
	p := NewDALLEProvider("test-key", "", nil)
	customLogger := slog.Default()
	p.SetLogger(customLogger)
	assert.Equal(t, customLogger, p.logger)
}

func TestDALLEProvider_SetLogger_Nil(t *testing.T) {
	p := NewDALLEProvider("test-key", "", nil)
	p.SetLogger(nil)
	assert.NotNil(t, p.logger)
}
