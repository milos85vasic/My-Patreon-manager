//go:build disabled

//go:build disabled\n
package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/middleware"
	ssync "github.com/milos85vasic/My-Patreon-Manager/internal/services/sync"
	"github.com/stretchr/testify/assert"
)

func TestWebhookSignature_InvalidRejected(t *testing.T) {
	secret := "webhook-secret"
	body := []byte(`{"ref":"refs/heads/main"}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	validSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	// Valid signature passes
	assert.True(t, middleware.ValidateGitHubSignature(body, validSig, secret))

	// Forged signature fails
	forgedSig := "sha256=" + hex.EncodeToString([]byte("forged"))
	assert.False(t, middleware.ValidateGitHubSignature(body, forgedSig, secret))

	// Wrong secret fails
	wrongSecret := "wrong-secret"
	mac2 := hmac.New(sha256.New, []byte(wrongSecret))
	mac2.Write(body)
	wrongSig := "sha256=" + hex.EncodeToString(mac2.Sum(nil))
	assert.False(t, middleware.ValidateGitHubSignature(body, wrongSig, secret))
}

func TestWebhookSignature_MissingSignature(t *testing.T) {
	secret := "webhook-secret"
	body := []byte(`{"ref":"refs/heads/main"}`)

	// Empty signature
	assert.False(t, middleware.ValidateGitHubSignature(body, "", secret))
	// Missing "sha256=" prefix
	assert.False(t, middleware.ValidateGitHubSignature(body, "abc123", secret))
	// Invalid hex encoding
	assert.False(t, middleware.ValidateGitHubSignature(body, "sha256=nothex", secret))
}

func TestWebhookSignature_TimingAttackResistance(t *testing.T) {
	// ValidateGitHubSignature uses hmac.Equal which is constant-time.
	// We cannot directly test timing, but we can verify that the function
	// uses the same code path regardless of signature correctness.
	secret := "secret"
	body := []byte("payload")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	validSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	// Generate a signature that differs in first byte
	sigBytes, _ := hex.DecodeString(validSig[7:])
	sigBytes[0] ^= 0x01
	wrongSig := "sha256=" + hex.EncodeToString(sigBytes)

	// Both calls should execute without panic and return appropriate bool
	assert.True(t, middleware.ValidateGitHubSignature(body, validSig, secret))
	assert.False(t, middleware.ValidateGitHubSignature(body, wrongSig, secret))
}

func TestWebhookSignature_ReplayPrevention(t *testing.T) {
	// Use the real EventDeduplicator
	window := 5 * time.Minute
	ed := ssync.NewEventDeduplicator(window)

	eventID := "event-123"
	assert.False(t, ed.IsDuplicate(eventID), "first event should not be duplicate")
	ed.TrackEvent(eventID)
	assert.True(t, ed.IsDuplicate(eventID), "second event should be duplicate")

	// Different event ID not duplicate
	assert.False(t, ed.IsDuplicate("event-456"))

	// After window expires, duplicate should be false
	// Simulate by creating a new deduplicator with short window
	shortEd := ssync.NewEventDeduplicator(50 * time.Millisecond)
	shortEd.TrackEvent("evt-1")
	assert.True(t, shortEd.IsDuplicate("evt-1"))
	time.Sleep(60 * time.Millisecond)
	assert.False(t, shortEd.IsDuplicate("evt-1"))
}

func TestGitLabTokenValidation(t *testing.T) {
	assert.True(t, middleware.ValidateGitLabToken("token", "token"))
	assert.False(t, middleware.ValidateGitLabToken("wrong", "token"))
	assert.False(t, middleware.ValidateGitLabToken("", "token"))
}

func TestGenericTokenValidation(t *testing.T) {
	assert.True(t, middleware.ValidateGenericToken("token", "token"))
	assert.False(t, middleware.ValidateGenericToken("wrong", "token"))
	assert.False(t, middleware.ValidateGenericToken("", "token"))
}

func TestWebhookAuthMiddleware_MissingSignature(t *testing.T) {
	// This test verifies that the middleware rejects requests with missing signature.
	// Since the middleware is integrated with Gin, we would need to spin up a test server.
	// For security test we rely on unit tests in handlers package.
	// We'll keep this as a placeholder to document the requirement.
	t.Skip("Middleware integration tested in unit/handlers/webhook_test.go")
}
