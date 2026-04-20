package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestSetupRouter_WebhookGenericService(t *testing.T) {
	cfg := &config.Config{
		GinMode:           "test",
		Port:              8080,
		AdminKey:          "admin-secret",
		WebhookHMACSecret: "webhook-secret",
		RateLimitRPS:      1000,
		RateLimitBurst:    2000,
	}
	router, dedup, _, _ := setupRouter(cfg, &mockMetricsCollector{}, noopOrchestrator{}, slog.Default(), nil)
	defer dedup.Close()

	// gitflic uses X-Webhook-Signature with HMAC
	body := []byte(`{"event":"push"}`)
	mac := hmac.New(sha256.New, []byte("webhook-secret"))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/webhook/gitflic", bytes.NewReader(body))
	req.Header.Set("X-Webhook-Signature", "sha256="+sig)
	router.ServeHTTP(w, req)
	// Should hit the GenericWebhook handler (not github/gitlab)
	assert.NotEqual(t, http.StatusUnauthorized, w.Code)
}

func TestSetupRouter_WebhookGitLab(t *testing.T) {
	cfg := &config.Config{
		GinMode:           "test",
		Port:              8080,
		AdminKey:          "admin-secret",
		WebhookHMACSecret: "webhook-secret",
		RateLimitRPS:      1000,
		RateLimitBurst:    2000,
	}
	router, dedup, _, _ := setupRouter(cfg, &mockMetricsCollector{}, noopOrchestrator{}, slog.Default(), nil)
	defer dedup.Close()

	body := []byte(`{"event":"push"}`)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/webhook/gitlab", bytes.NewReader(body))
	req.Header.Set("X-Gitlab-Token", "webhook-secret")
	router.ServeHTTP(w, req)
	assert.NotEqual(t, http.StatusUnauthorized, w.Code)
}

// --- serverBuildImageProviders coverage ---
//
// The server's image-provider factory takes a config and returns a slice of
// ImageProvider implementations based on which credentials are populated.
// The tests below exercise each branch (empty → zero providers, every key
// populated → all four providers) so the function is fully covered without
// making any real network calls — ImageProvider constructors are pure
// struct-initialization.

func TestServerBuildImageProviders_AllEmpty(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

	got := serverBuildImageProviders(cfg, logger)
	assert.Empty(t, got, "empty config must yield no providers")
}

func TestServerBuildImageProviders_AllPopulated(t *testing.T) {
	cfg := &config.Config{
		OpenAIAPIKey:        "dall-key",
		OpenAIBaseURL:       "https://dalle.example/v1",
		StabilityAIAPIKey:   "stability-key",
		StabilityAIBaseURL:  "https://stability.example/v1",
		MidjourneyAPIKey:    "mj-key",
		MidjourneyEndpoint:  "https://mj.example/api",
		OpenAICompatAPIKey:  "compat-key",
		OpenAICompatBaseURL: "https://compat.example/v1",
		OpenAICompatModel:   "dall-compat-1",
	}
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

	got := serverBuildImageProviders(cfg, logger)
	assert.Len(t, got, 4, "every populated credential must yield its provider")
}

func TestServerBuildImageProviders_PartialPopulated(t *testing.T) {
	cfg := &config.Config{
		OpenAIAPIKey:       "dall-key",
		MidjourneyAPIKey:   "mj-key",
		MidjourneyEndpoint: "https://mj.example/api",
	}
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

	got := serverBuildImageProviders(cfg, logger)
	assert.Len(t, got, 2, "two populated credentials must yield exactly two providers")
}
