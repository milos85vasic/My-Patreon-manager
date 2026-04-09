package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/gin-gonic/gin"
)

func ValidateGitHubSignature(body []byte, signature string, secret string) bool {
	if !strings.HasPrefix(signature, "sha256=") {
		return false
	}
	sig, err := hex.DecodeString(signature[7:])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := mac.Sum(nil)
	return hmac.Equal(sig, expected)
}

func ValidateGitLabToken(token, expected string) bool {
	return token != "" && token == expected
}

func ValidateGenericToken(token, expected string) bool {
	return token != "" && token == expected
}

func WebhookAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		service := c.Param("service")
		switch service {
		case "github":
			signature := c.GetHeader("X-Hub-Signature-256")
			if !ValidateGitHubSignature(nil, signature, secret) {
				c.AbortWithStatusJSON(401, gin.H{"error": "invalid signature"})
				return
			}
		case "gitlab":
			token := c.GetHeader("X-Gitlab-Token")
			if !ValidateGitLabToken(token, secret) {
				c.AbortWithStatusJSON(401, gin.H{"error": "invalid token"})
				return
			}
		default:
			token := c.GetHeader("X-Webhook-Token")
			if !ValidateGenericToken(token, secret) {
				c.AbortWithStatusJSON(401, gin.H{"error": "invalid token"})
				return
			}
		}
		c.Next()
	}
}
