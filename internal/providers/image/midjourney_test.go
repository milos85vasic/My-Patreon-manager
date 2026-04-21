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

func TestMidjourneyProvider_ProviderName(t *testing.T) {
	p := NewMidjourneyProvider("test-key", "http://localhost", nil)
	assert.Equal(t, "midjourney", p.ProviderName())
}

func TestMidjourneyProvider_IsAvailable(t *testing.T) {
	p := NewMidjourneyProvider("test-key", "http://localhost", nil)
	assert.True(t, p.IsAvailable(context.Background()))

	p2 := NewMidjourneyProvider("", "", nil)
	assert.False(t, p2.IsAvailable(context.Background()))

	p3 := NewMidjourneyProvider("key", "", nil)
	assert.False(t, p3.IsAvailable(context.Background()))

	p4 := NewMidjourneyProvider("", "http://localhost", nil)
	assert.False(t, p4.IsAvailable(context.Background()))
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

func TestMidjourneyProvider_GenerateImage_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "internal error"})
	}))
	defer server.Close()

	p := NewMidjourneyProvider("test-key", server.URL, server.Client())

	result, err := p.GenerateImage(context.Background(), ImageRequest{Prompt: "test"})
	assert.Nil(t, result)
	assert.Error(t, err)
}

func TestMidjourneyProvider_GenerateImage_NotConfigured(t *testing.T) {
	p := NewMidjourneyProvider("", "", nil)
	result, err := p.GenerateImage(context.Background(), ImageRequest{Prompt: "test"})
	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "API key or endpoint not configured")
}

func TestMidjourneyProvider_SetLogger(t *testing.T) {
	p := NewMidjourneyProvider("test-key", "http://localhost", nil)
	customLogger := slog.Default()
	p.SetLogger(customLogger)
	assert.Equal(t, customLogger, p.logger)
}

func TestMidjourneyProvider_SetLogger_Nil(t *testing.T) {
	p := NewMidjourneyProvider("test-key", "http://localhost", nil)
	p.SetLogger(nil)
	assert.NotNil(t, p.logger)
}
