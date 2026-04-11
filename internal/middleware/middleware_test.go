package middleware

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestLogger(t *testing.T) {
	engine := gin.New()
	engine.Use(Logger())
	engine.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest("GET", "/test?token=abc", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuth_AdminKeyProvided(t *testing.T) {
	engine := gin.New()
	engine.Use(Auth("secret-admin-key"))
	engine.GET("/admin/dashboard", func(c *gin.Context) {
		c.String(http.StatusOK, "admin")
	})
	engine.GET("/public", func(c *gin.Context) {
		c.String(http.StatusOK, "public")
	})

	// Admin path without key -> 401
	req := httptest.NewRequest("GET", "/admin/dashboard", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// Admin path with wrong key -> 403
	req = httptest.NewRequest("GET", "/admin/dashboard", nil)
	req.Header.Set("X-Admin-Key", "wrong-key")
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)

	// Admin path with correct key -> 200
	req = httptest.NewRequest("GET", "/admin/dashboard", nil)
	req.Header.Set("X-Admin-Key", "secret-admin-key")
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "admin", w.Body.String())

	// Non-admin path works without key
	req = httptest.NewRequest("GET", "/public", nil)
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "public", w.Body.String())
}

func TestAuth_AdminKeyFromEnv(t *testing.T) {
	os.Setenv("ADMIN_KEY", "env-admin-key")
	defer os.Unsetenv("ADMIN_KEY")

	engine := gin.New()
	// Pass empty string to trigger env fallback
	engine.Use(Auth(""))
	engine.GET("/admin/dashboard", func(c *gin.Context) {
		c.String(http.StatusOK, "admin")
	})

	// Should work with env key
	req := httptest.NewRequest("GET", "/admin/dashboard", nil)
	req.Header.Set("X-Admin-Key", "env-admin-key")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Wrong key should fail
	req = httptest.NewRequest("GET", "/admin/dashboard", nil)
	req.Header.Set("X-Admin-Key", "wrong")
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestRateLimit(t *testing.T) {
	// Create a rate limiter with 1 request per second, burst 1
	engine := gin.New()
	engine.Use(RateLimit(1, 1))
	engine.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	// First request should succeed
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Second request from same IP should be rate limited
	req = httptest.NewRequest("GET", "/test", nil)
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Contains(t, w.Body.String(), "rate limit exceeded")
	assert.Equal(t, "60", w.Header().Get("Retry-After"))

	// Different IP should succeed
	req = httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.2:12345"
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRecovery(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	engine := gin.New()
	engine.Use(Recovery(logger))
	engine.GET("/panic", func(c *gin.Context) {
		panic("test panic")
	})
	engine.GET("/normal", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	// Panic route should return 500 and not crash
	req := httptest.NewRequest("GET", "/panic", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "internal server error")

	// Normal route should work
	req = httptest.NewRequest("GET", "/normal", nil)
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRecovery_NilLogger(t *testing.T) {
	engine := gin.New()
	engine.Use(Recovery(nil))
	engine.GET("/panic", func(c *gin.Context) {
		panic("test panic")
	})

	req := httptest.NewRequest("GET", "/panic", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestValidateGitHubSignature(t *testing.T) {
	secret := "mysecret"
	body := []byte(`{"event":"push"}`)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expectedSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	// Valid signature
	assert.True(t, ValidateGitHubSignature(body, expectedSig, secret))

	// Invalid signature
	assert.False(t, ValidateGitHubSignature(body, "sha256=invalid", secret))

	// Missing prefix
	assert.False(t, ValidateGitHubSignature(body, "invalid", secret))

	// Malformed hex
	assert.False(t, ValidateGitHubSignature(body, "sha256=nothex", secret))
}

func TestValidateGitLabToken(t *testing.T) {
	assert.True(t, ValidateGitLabToken("token", "token"))
	assert.False(t, ValidateGitLabToken("token", "wrong"))
	assert.False(t, ValidateGitLabToken("", "token"))
}

func TestValidateGenericToken(t *testing.T) {
	assert.True(t, ValidateGenericToken("token", "token"))
	assert.False(t, ValidateGenericToken("token", "wrong"))
	assert.False(t, ValidateGenericToken("", "token"))
}

func TestWebhookAuth(t *testing.T) {
	secret := "webhook-secret"
	engine := gin.New()
	engine.Use(WebhookAuth(secret))
	engine.POST("/webhook/:service", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	// GitHub webhook with valid signature
	body := []byte(`{"event":"push"}`)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	req := httptest.NewRequest("POST", "/webhook/github", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sig)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// GitHub webhook with invalid signature -> 401
	req = httptest.NewRequest("POST", "/webhook/github", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", "sha256=invalid")
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// GitLab webhook with valid token
	req = httptest.NewRequest("POST", "/webhook/gitlab", nil)
	req.Header.Set("X-Gitlab-Token", secret)
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// GitLab webhook with invalid token -> 401
	req = httptest.NewRequest("POST", "/webhook/gitlab", nil)
	req.Header.Set("X-Gitlab-Token", "wrong")
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// Generic webhook with valid token
	req = httptest.NewRequest("POST", "/webhook/other", nil)
	req.Header.Set("X-Webhook-Token", secret)
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Generic webhook with invalid token -> 401
	req = httptest.NewRequest("POST", "/webhook/other", nil)
	req.Header.Set("X-Webhook-Token", "wrong")
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// Missing token -> 401
	req = httptest.NewRequest("POST", "/webhook/other", nil)
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestIPRateLimiter_CleanupStale(t *testing.T) {
	// Fresh entries (age below maxAge) are preserved.
	limiter := NewIPRateLimiter(1, 5) // rate 1 per second, burst 5
	ip := "192.168.1.1"
	l := limiter.GetLimiter(ip)
	assert.True(t, l.Allow())
	assert.True(t, l.Allow())
	limiter.CleanupStale(time.Hour)
	rv := reflect.ValueOf(limiter).Elem()
	limitersField := rv.FieldByName("limiters")
	entry := limitersField.MapIndex(reflect.ValueOf(ip))
	assert.True(t, entry.IsValid(), "fresh limiter should not be removed")

	// Entries older than maxAge are evicted (maxAge=0 sweeps using TTL).
	limiter2 := NewIPRateLimiter(1, 1, 1*time.Nanosecond)
	ip2 := "192.168.1.2"
	_ = limiter2.GetLimiter(ip2)
	time.Sleep(5 * time.Millisecond)
	limiter2.CleanupStale(0)
	rv2 := reflect.ValueOf(limiter2).Elem()
	limitersField2 := rv2.FieldByName("limiters")
	entry2 := limitersField2.MapIndex(reflect.ValueOf(ip2))
	assert.False(t, entry2.IsValid(), "stale limiter should be removed")
}

func TestWebhookAuth_ErrorReadingBody(t *testing.T) {
	// Create a custom reader that returns error
	brokenReader := &errorReader{}
	// Create a gin context with the broken reader
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/webhook/github", brokenReader)
	c.Params = gin.Params{gin.Param{Key: "service", Value: "github"}}
	c.Set("secret", "secret")
	// Call the middleware directly
	handler := WebhookAuth("secret")
	handler(c)
	// Should have aborted with 401
	assert.Equal(t, 401, c.Writer.Status())
}

type errorReader struct{}

func (e *errorReader) Read(p []byte) (n int, err error) {
	return 0, io.ErrUnexpectedEOF
}

func (e *errorReader) Close() error {
	return nil
}
