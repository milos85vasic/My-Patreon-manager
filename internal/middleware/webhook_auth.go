package middleware

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func ValidateGitHubSignature(body []byte, signature string, secret string) bool {
	return verifyHMACSignature(signature, secret, body)
}

func ValidateGitLabToken(token, expected string) bool {
	return token != "" && subtle.ConstantTimeCompare([]byte(token), []byte(expected)) == 1
}

func ValidateGenericSignature(body []byte, signature string, secret string) bool {
	return verifyHMACSignature(signature, secret, body)
}

// ValidateGenericToken is the legacy token-based validator. Kept for backward
// compatibility; new code should use ValidateGenericSignature for HMAC checks.
func ValidateGenericToken(token, expected string) bool {
	return token != "" && subtle.ConstantTimeCompare([]byte(token), []byte(expected)) == 1
}

func verifyHMACSignature(header, secret string, body []byte) bool {
	if secret == "" || header == "" {
		return false
	}
	const prefix = "sha256="
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	given, err := hex.DecodeString(header[len(prefix):])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return subtle.ConstantTimeCompare(given, mac.Sum(nil)) == 1
}

func WebhookAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		service := c.Param("service")
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(body))

		var ok bool
		switch service {
		case "github":
			ok = ValidateGitHubSignature(body, c.GetHeader("X-Hub-Signature-256"), secret)
		case "gitlab":
			ok = ValidateGitLabToken(c.GetHeader("X-Gitlab-Token"), secret)
		case "gitflic", "gitverse":
			ok = ValidateGenericSignature(body, c.GetHeader("X-Webhook-Signature"), secret)
		default:
			ok = false
		}

		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
			return
		}
		c.Next()
	}
}
