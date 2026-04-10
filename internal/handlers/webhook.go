package handlers

import (
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/milos85vasic/My-Patreon-Manager/internal/metrics"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/sync"
)

func splitFullName(full string) (owner, name string) {
	parts := strings.Split(full, "/")
	if len(parts) >= 2 {
		owner = parts[0]
		name = parts[1]
	}
	return
}

type WebhookHandler struct {
	dedup   *sync.EventDeduplicator
	metrics metrics.MetricsCollector
	logger  *slog.Logger
	Queue   chan models.Repository // optional queue for repository processing
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

	// Parse repository from webhook payload
	var payload struct {
		Repository struct {
			FullName string `json:"full_name"`
			HTMLURL  string `json:"html_url"`
		} `json:"repository"`
	}
	body, err := c.GetRawData()
	if err == nil && len(body) > 0 {
		if err := json.Unmarshal(body, &payload); err == nil && payload.Repository.FullName != "" {
			// Extract owner and name
			owner, name := splitFullName(payload.Repository.FullName)
			repo := models.Repository{
				ID:       payload.Repository.FullName,
				Service:  "github",
				Owner:    owner,
				Name:     name,
				HTTPSURL: payload.Repository.HTMLURL,
			}
			// Send to queue if configured
			if h.Queue != nil {
				select {
				case h.Queue <- repo:
				default:
					// Queue full, log warning
					if h.logger != nil {
						h.logger.Warn("webhook queue full, dropping repository", slog.String("repo", repo.ID))
					}
				}
			}
		}
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

	// Parse repository from webhook payload
	var payload struct {
		Project struct {
			PathWithNamespace string `json:"path_with_namespace"`
			WebURL            string `json:"web_url"`
		} `json:"project"`
	}
	body, err := c.GetRawData()
	if err == nil && len(body) > 0 {
		if err := json.Unmarshal(body, &payload); err == nil && payload.Project.PathWithNamespace != "" {
			// Extract owner and name
			owner, name := splitFullName(payload.Project.PathWithNamespace)
			repo := models.Repository{
				ID:       payload.Project.PathWithNamespace,
				Service:  "gitlab",
				Owner:    owner,
				Name:     name,
				HTTPSURL: payload.Project.WebURL,
			}
			// Send to queue if configured
			if h.Queue != nil {
				select {
				case h.Queue <- repo:
				default:
					// Queue full, log warning
					if h.logger != nil {
						h.logger.Warn("webhook queue full, dropping repository", slog.String("repo", repo.ID))
					}
				}
			}
		}
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
