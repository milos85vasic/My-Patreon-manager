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

func TestStabilityProvider_ProviderName(t *testing.T) {
	p := NewStabilityProvider("test-key", "", nil)
	assert.Equal(t, "stability", p.ProviderName())
}

func TestStabilityProvider_IsAvailable(t *testing.T) {
	p := NewStabilityProvider("test-key", "", nil)
	assert.True(t, p.IsAvailable(context.Background()))

	p2 := NewStabilityProvider("", "", nil)
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

	p := NewStabilityProvider("test-key", server.URL, server.Client())

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

	p := NewStabilityProvider("test-key", server.URL, server.Client())

	result, err := p.GenerateImage(context.Background(), ImageRequest{
		Prompt: "test",
	})
	assert.Nil(t, result)
	assert.Error(t, err)
}

func TestStabilityProvider_GenerateImage_NoAPIKey(t *testing.T) {
	p := NewStabilityProvider("", "", nil)
	result, err := p.GenerateImage(context.Background(), ImageRequest{Prompt: "test"})
	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "API key not configured")
}

// TestStabilityProvider_CustomBaseURL confirms a custom base URL passed
// to the constructor is honored for the request path.
func TestStabilityProvider_CustomBaseURL(t *testing.T) {
	var gotURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.Path
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte{0x89})
	}))
	defer server.Close()

	p := NewStabilityProvider("test-key", server.URL+"/v9", server.Client())
	_, err := p.GenerateImage(context.Background(), ImageRequest{Prompt: "p"})
	require.NoError(t, err)
	assert.Equal(t, "/v9/stable-image/generate/sdxl", gotURL)
	assert.Equal(t, server.URL+"/v9", p.baseURL)
}

// TestStabilityProvider_EmptyBaseURLFallsBack confirms the public
// Stability endpoint default is applied when no base URL is supplied.
func TestStabilityProvider_EmptyBaseURLFallsBack(t *testing.T) {
	p := NewStabilityProvider("test-key", "", nil)
	assert.Equal(t, "https://api.stability.ai/v2beta", p.baseURL)
}

func TestStabilityProvider_SetLogger(t *testing.T) {
	p := NewStabilityProvider("test-key", "", nil)
	customLogger := slog.Default()
	p.SetLogger(customLogger)
	assert.Equal(t, customLogger, p.logger)
}

func TestStabilityProvider_SetLogger_Nil(t *testing.T) {
	p := NewStabilityProvider("test-key", "", nil)
	p.SetLogger(nil)
	assert.NotNil(t, p.logger)
}
