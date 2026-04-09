package handlers

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/milos85vasic/My-Patreon-Manager/internal/metrics"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/sync"
)

type WebhookHandler struct {
	dedup   *sync.EventDeduplicator
	metrics metrics.MetricsCollector
	logger  *slog.Logger
}

func NewWebhookHandler(dedup *sync.EventDeduplicator, m metrics.MetricsCollector, logger *slog.Logger) *WebhookHandler {
	return &WebhookHandler{dedup: dedup, metrics: m, logger: logger}
}

func (h *WebhookHandler) GitHubWebhook(c *gin.Context) {
	eventID := c.GetHeader("X-GitHub-Delivery")
	eventType := c.GetHeader("X-GitHub-Event")

	if h.dedup != nil {
		if h.dedup.IsDuplicate(eventID) {
			c.JSON(200, gin.H{"status": "duplicate_ignored"})
			return
		}
		h.dedup.TrackEvent(eventID)
	}

	if h.metrics != nil {
		h.metrics.RecordWebhookEvent("github", eventType)
	}

	if h.logger != nil {
		h.logger.Info("github webhook received", slog.String("event", eventType), slog.String("delivery", eventID))
	}

	c.JSON(200, gin.H{"status": "queued", "event": eventType})
}

func (h *WebhookHandler) GitLabWebhook(c *gin.Context) {
	eventType := c.GetHeader("X-Gitlab-Event")
	eventID := c.GetHeader("X-Gitlab-Token")

	if h.dedup != nil {
		if h.dedup.IsDuplicate(eventID) {
			c.JSON(200, gin.H{"status": "duplicate_ignored"})
			return
		}
		h.dedup.TrackEvent(eventID)
	}

	if h.metrics != nil {
		h.metrics.RecordWebhookEvent("gitlab", eventType)
	}

	if h.logger != nil {
		h.logger.Info("gitlab webhook received", slog.String("event", eventType))
	}

	c.JSON(200, gin.H{"status": "queued", "event": eventType})
}

func (h *WebhookHandler) GenericWebhook(c *gin.Context) {
	service := c.Param("service")

	if h.metrics != nil {
		h.metrics.RecordWebhookEvent(service, "push")
	}

	if h.logger != nil {
		h.logger.Info("webhook received", slog.String("service", service))
	}

	c.JSON(200, gin.H{"status": "queued", "service": service})
}
