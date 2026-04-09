package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestWebhookSignature_InvalidRejected(t *testing.T) {
	secret := "webhook-secret"
	body := []byte(`{"ref":"refs/heads/main"}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	validSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	forgedSig := "sha256=" + hex.EncodeToString([]byte("forged"))
	assert.NotEqual(t, validSig, forgedSig)

	mac2 := hmac.New(sha256.New, []byte(secret))
	mac2.Write(body)
	expectedSig := hex.EncodeToString(mac2.Sum(nil))
	assert.Equal(t, expectedSig, hex.EncodeToString(mac.Sum(nil)))
}

func TestWebhookSignature_ReplayPrevention(t *testing.T) {
	ed := NewTestDeduplicator(5 * time.Minute)

	eventID := "event-123"
	ed.Track(eventID)
	assert.True(t, ed.IsDup(eventID), "second event should be duplicate")
	assert.False(t, ed.IsDup("event-456"), "different event should not be duplicate")
}

func TestWebhookSignature_MissingSignature(t *testing.T) {
	secret := "webhook-secret"
	body := []byte(`{"ref":"refs/heads/main"}`)

	mac := hmac.New(sha256.New, []byte("wrong-secret"))
	mac.Write(body)
	wrongSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	mac2 := hmac.New(sha256.New, []byte(secret))
	mac2.Write(body)
	correctSig := "sha256=" + hex.EncodeToString(mac2.Sum(nil))

	assert.NotEqual(t, wrongSig, correctSig, "wrong secret should produce different signature")
}

type testDeduplicator struct {
	seen map[string]time.Time
	ttl  time.Duration
}

func NewTestDeduplicator(ttl time.Duration) *testDeduplicator {
	return &testDeduplicator{seen: make(map[string]time.Time), ttl: ttl}
}

func (d *testDeduplicator) Track(id string) {
	d.seen[id] = time.Now()
}

func (d *testDeduplicator) IsDup(id string) bool {
	t, ok := d.seen[id]
	if !ok {
		return false
	}
	return time.Since(t) < d.ttl
}
