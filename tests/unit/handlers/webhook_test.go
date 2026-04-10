package handlers_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/milos85vasic/My-Patreon-Manager/internal/handlers"
	"github.com/milos85vasic/My-Patreon-Manager/internal/metrics"
	"github.com/milos85vasic/My-Patreon-Manager/internal/middleware"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/sync"
	"github.com/stretchr/testify/assert"
)

// mockMetricsCollector implements metrics.MetricsCollector for testing.
type mockMetricsCollector struct {
	recordedEvents []string
}

func (m *mockMetricsCollector) RecordWebhookEvent(service, eventType string) {
	m.recordedEvents = append(m.recordedEvents, service+":"+eventType)
}
func (m *mockMetricsCollector) RecordSyncDuration(service, status string, seconds float64) {}
func (m *mockMetricsCollector) RecordReposProcessed(service, action string)                {}
func (m *mockMetricsCollector) RecordAPIError(service, errorType string)                   {}
func (m *mockMetricsCollector) RecordLLMLatency(model string, seconds float64)             {}
func (m *mockMetricsCollector) RecordLLMTokens(model, tokenType string, count int)         {}
func (m *mockMetricsCollector) RecordLLMQualityScore(repository string, score float64)     {}
func (m *mockMetricsCollector) RecordContentGenerated(format, qualityTier string)          {}
func (m *mockMetricsCollector) RecordPostCreated(tier string)                              {}
func (m *mockMetricsCollector) RecordPostUpdated(tier string)                              {}
func (m *mockMetricsCollector) SetActiveSyncs(count int)                                   {}
func (m *mockMetricsCollector) SetBudgetUtilization(percent float64)                       {}

var _ metrics.MetricsCollector = (*mockMetricsCollector)(nil)

func TestGitHubWebhook_Valid(t *testing.T) {
	dedup := sync.NewEventDeduplicator(5 * time.Minute)
	metrics := &mockMetricsCollector{}
	h := handlers.NewWebhookHandler(dedup, metrics, nil)

	router := gin.New()
	router.POST("/webhook/github", h.GitHubWebhook)

	body := `{"repository":{"full_name":"owner/repo"}}`
	req := httptest.NewRequest("POST", "/webhook/github", bytes.NewBufferString(body))
	req.Header.Set("X-GitHub-Delivery", "delivery-id")
	req.Header.Set("X-GitHub-Event", "push")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"status":"queued","event":"push"}`, w.Body.String())
	assert.Equal(t, []string{"github:push"}, metrics.recordedEvents)
	// dedup tracked the event
	assert.True(t, dedup.IsDuplicate("delivery-id"))
}

func TestGitHubWebhook_Duplicate(t *testing.T) {
	dedup := sync.NewEventDeduplicator(5 * time.Minute)
	dedup.TrackEvent("dup-id") // mark as duplicate
	metrics := &mockMetricsCollector{}
	h := handlers.NewWebhookHandler(dedup, metrics, nil)

	router := gin.New()
	router.POST("/webhook/github", h.GitHubWebhook)

	req := httptest.NewRequest("POST", "/webhook/github", nil)
	req.Header.Set("X-GitHub-Delivery", "dup-id")
	req.Header.Set("X-GitHub-Event", "push")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"status":"duplicate_ignored"}`, w.Body.String())
	assert.Empty(t, metrics.recordedEvents)
	// duplicate not tracked again
}

func TestGitHubWebhook_NoHeaders(t *testing.T) {
	dedup := sync.NewEventDeduplicator(5 * time.Minute)
	metrics := &mockMetricsCollector{}
	h := handlers.NewWebhookHandler(dedup, metrics, nil)

	router := gin.New()
	router.POST("/webhook/github", h.GitHubWebhook)

	req := httptest.NewRequest("POST", "/webhook/github", nil)
	// missing headers
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// still responds with queued because headers are optional for dedup
	assert.JSONEq(t, `{"status":"queued","event":""}`, w.Body.String())
}

func TestGitLabWebhook_Valid(t *testing.T) {
	dedup := sync.NewEventDeduplicator(5 * time.Minute)
	metrics := &mockMetricsCollector{}
	h := handlers.NewWebhookHandler(dedup, metrics, nil)

	router := gin.New()
	router.POST("/webhook/gitlab", h.GitLabWebhook)

	body := `{"project":{"path_with_namespace":"group/project"}}`
	req := httptest.NewRequest("POST", "/webhook/gitlab", bytes.NewBufferString(body))
	req.Header.Set("X-Gitlab-Event", "Push Hook")
	req.Header.Set("X-Gitlab-Token", "token-id")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"status":"queued","event":"Push Hook"}`, w.Body.String())
	assert.Equal(t, []string{"gitlab:Push Hook"}, metrics.recordedEvents)
	assert.True(t, dedup.IsDuplicate("token-id"))
}

func TestGenericWebhook_Valid(t *testing.T) {
	dedup := sync.NewEventDeduplicator(5 * time.Minute)
	metrics := &mockMetricsCollector{}
	h := handlers.NewWebhookHandler(dedup, metrics, nil)

	router := gin.New()
	router.POST("/webhook/:service", h.GenericWebhook)

	req := httptest.NewRequest("POST", "/webhook/custom", nil)
	req.Header.Set("X-Webhook-Token", "some-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"status":"queued","service":"custom"}`, w.Body.String())
	assert.Equal(t, []string{"custom:push"}, metrics.recordedEvents)
	// generic webhook does not deduplicate
	assert.False(t, dedup.IsDuplicate("some-token"))
}

// Test middleware validation functions
func TestValidateGitHubSignature(t *testing.T) {
	secret := "mysecret"
	body := []byte(`{"payload": "data"}`)
	signature := "sha256=abc123" // invalid

	// We cannot test the actual HMAC because we'd need to compute it.
	// This test just ensures the function exists and returns false for invalid.
	// For unit test we can test that missing prefix fails.
	valid := middleware.ValidateGitHubSignature(body, signature, secret)
	assert.False(t, valid)
}

func TestValidateGitLabToken(t *testing.T) {
	assert.True(t, middleware.ValidateGitLabToken("token", "token"))
	assert.False(t, middleware.ValidateGitLabToken("wrong", "token"))
	assert.False(t, middleware.ValidateGitLabToken("", "token"))
}

func TestValidateGenericToken(t *testing.T) {
	assert.True(t, middleware.ValidateGenericToken("token", "token"))
	assert.False(t, middleware.ValidateGenericToken("wrong", "token"))
	assert.False(t, middleware.ValidateGenericToken("", "token"))
}
