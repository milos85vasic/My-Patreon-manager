package integration

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/handlers"
	"github.com/milos85vasic/My-Patreon-Manager/internal/metrics"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebhookFlow_GitHub(t *testing.T) {
	db := setupWebhookDB(t)
	defer db.Close()

	// Create a channel to capture queued repositories
	queue := make(chan models.Repository, 10)
	dedup := sync.NewEventDeduplicator(5 * time.Minute)
	mc := &webhookMockMetricsCollector{}
	h := handlers.NewWebhookHandler(dedup, mc, nil)
	h.Queue = queue

	router := gin.New()
	router.POST("/webhook/github", h.GitHubWebhook)

	// GitHub push event payload
	body := `{
		"repository": {
			"full_name": "owner/repo",
			"html_url": "https://github.com/owner/repo"
		}
	}`
	req := httptest.NewRequest("POST", "/webhook/github", bytes.NewBufferString(body))
	req.Header.Set("X-GitHub-Delivery", "test-delivery")
	req.Header.Set("X-GitHub-Event", "push")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"status":"queued","event":"push"}`, w.Body.String())

	// Verify repository was queued
	select {
	case repo := <-queue:
		assert.Equal(t, "owner/repo", repo.ID)
		assert.Equal(t, "github", repo.Service)
		assert.Equal(t, "owner", repo.Owner)
		assert.Equal(t, "repo", repo.Name)
		assert.Equal(t, "https://github.com/owner/repo", repo.HTTPSURL)
	case <-time.After(1 * time.Second):
		t.Fatal("expected repository to be queued within 1 second")
	}

	// Verify deduplication tracked the event
	assert.True(t, dedup.IsDuplicate("test-delivery"))
	assert.Equal(t, []string{"github:push"}, mc.recordedEvents)
}

func TestWebhookFlow_GitLab(t *testing.T) {
	db := setupWebhookDB(t)
	defer db.Close()

	queue := make(chan models.Repository, 10)
	dedup := sync.NewEventDeduplicator(5 * time.Minute)
	mc := &webhookMockMetricsCollector{}
	h := handlers.NewWebhookHandler(dedup, mc, nil)
	h.Queue = queue

	router := gin.New()
	router.POST("/webhook/gitlab", h.GitLabWebhook)

	body := `{
		"project": {
			"path_with_namespace": "group/project",
			"web_url": "https://gitlab.com/group/project"
		}
	}`
	req := httptest.NewRequest("POST", "/webhook/gitlab", bytes.NewBufferString(body))
	req.Header.Set("X-Gitlab-Event", "Push Hook")
	req.Header.Set("X-Gitlab-Token", "token-id")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"status":"queued","event":"Push Hook"}`, w.Body.String())

	select {
	case repo := <-queue:
		assert.Equal(t, "group/project", repo.ID)
		assert.Equal(t, "gitlab", repo.Service)
		assert.Equal(t, "group", repo.Owner)
		assert.Equal(t, "project", repo.Name)
		assert.Equal(t, "https://gitlab.com/group/project", repo.HTTPSURL)
	case <-time.After(1 * time.Second):
		t.Fatal("expected repository to be queued within 1 second")
	}

	assert.True(t, dedup.IsDuplicate("token-id"))
	assert.Equal(t, []string{"gitlab:Push Hook"}, mc.recordedEvents)
}

// webhookMockMetricsCollector from unit test
type webhookMockMetricsCollector struct {
	recordedEvents []string
}

func (m *webhookMockMetricsCollector) RecordWebhookEvent(service, eventType string) {
	m.recordedEvents = append(m.recordedEvents, service+":"+eventType)
}
func (m *webhookMockMetricsCollector) RecordSyncDuration(service, status string, seconds float64) {}
func (m *webhookMockMetricsCollector) RecordReposProcessed(service, action string)                {}
func (m *webhookMockMetricsCollector) RecordAPIError(service, errorType string)                   {}
func (m *webhookMockMetricsCollector) RecordLLMLatency(model string, seconds float64)             {}
func (m *webhookMockMetricsCollector) RecordLLMTokens(model, tokenType string, count int)         {}
func (m *webhookMockMetricsCollector) RecordLLMQualityScore(repository string, score float64)     {}
func (m *webhookMockMetricsCollector) RecordContentGenerated(format, qualityTier string)          {}
func (m *webhookMockMetricsCollector) RecordPostCreated(tier string)                              {}
func (m *webhookMockMetricsCollector) RecordPostUpdated(tier string)                              {}
func (m *webhookMockMetricsCollector) SetActiveSyncs(count int)                                   {}
func (m *webhookMockMetricsCollector) SetBudgetUtilization(percent float64)                       {}

var _ metrics.MetricsCollector = (*webhookMockMetricsCollector)(nil)

// setupWebhookDB copied from other integration tests
func setupWebhookDB(t *testing.T) *database.SQLiteDB {
	db := database.NewSQLiteDB(":memory:")
	err := db.Connect(context.Background(), "")
	require.NoError(t, err)
	err = db.Migrate(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}
