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
