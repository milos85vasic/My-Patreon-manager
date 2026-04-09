package middleware

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

func Auth(adminKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if adminKey == "" {
			adminKey = os.Getenv("ADMIN_KEY")
		}

		path := c.Request.URL.Path
		if len(path) >= 7 && path[:7] == "/admin/" {
			key := c.GetHeader("X-Admin-Key")
			if key == "" {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "admin key required"})
				return
			}
			if key != adminKey {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "invalid admin key"})
				return
			}
		}

		c.Next()
	}
}
