package handlers

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/access"
)

type AccessHandler struct {
	gater  *access.TierGater
	urlGen *access.SignedURLGenerator
	logger *slog.Logger
}

func NewAccessHandler(gater *access.TierGater, urlGen *access.SignedURLGenerator, logger *slog.Logger) *AccessHandler {
	return &AccessHandler{gater: gater, urlGen: urlGen, logger: logger}
}

func (h *AccessHandler) Download(c *gin.Context) {
	contentID := c.Param("content_id")
	token := c.Query("token")
	sub := c.Query("sub")
	expStr := c.Query("exp")

	if token == "" || sub == "" || expStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing token parameters"})
		return
	}

	expires, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid expiry"})
		return
	}

	if !h.urlGen.VerifySignedURL(token, contentID, sub, expires) {
		c.JSON(http.StatusForbidden, gin.H{"error": "invalid or expired token"})
		return
	}

	c.Header("Content-Disposition", "attachment; filename="+contentID)
	c.JSON(http.StatusOK, gin.H{"content_id": contentID, "status": "download_ready"})
}

func (h *AccessHandler) CheckAccess(c *gin.Context) {
	contentID := c.Param("content_id")
	patronID := c.Query("patron_id")
	requiredTier := c.Query("required_tier")

	if patronID == "" || requiredTier == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing patron_id or required_tier"})
		return
	}

	hasAccess, upgradeURL, _ := h.gater.VerifyAccess(c.Request.Context(), patronID, contentID, requiredTier, nil)

	response := gin.H{
		"access":        hasAccess,
		"content_id":    contentID,
		"required_tier": requiredTier,
	}

	if !hasAccess {
		response["upgrade_url"] = upgradeURL
	}

	c.JSON(http.StatusOK, response)
}
