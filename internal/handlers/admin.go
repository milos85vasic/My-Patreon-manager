package handlers

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

func AdminReload(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "config_reloaded"})
}

func AdminSyncStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":      "idle",
		"active_sync": false,
	})
}

func RegisterAdminRoutes(r *gin.Engine, logger *slog.Logger) {
	admin := r.Group("/admin")
	{
		admin.POST("/reload", AdminReload)
		admin.GET("/sync/status", AdminSyncStatus)
	}
}
