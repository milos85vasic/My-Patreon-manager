package access

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type SignedURLGenerator struct {
	secret string
	ttl    time.Duration
}

func NewSignedURLGenerator(secret string, ttl time.Duration) *SignedURLGenerator {
	return &SignedURLGenerator{secret: secret, ttl: ttl}
}

func (g *SignedURLGenerator) GenerateSignedURL(contentID, subscriberID string) string {
	expires := time.Now().Add(g.ttl).Unix()
	payload := fmt.Sprintf("%s:%s:%d", contentID, subscriberID, expires)
	mac := hmac.New(sha256.New, []byte(g.secret))
	mac.Write([]byte(payload))
	sig := hex.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("/download/%s?token=%s&sub=%s&exp=%d", contentID, sig, subscriberID, expires)
}

func (g *SignedURLGenerator) VerifySignedURL(token, contentID, subscriberID string, expires int64) bool {
	if time.Now().Unix() > expires {
		return false
	}
	payload := fmt.Sprintf("%s:%s:%d", contentID, subscriberID, expires)
	mac := hmac.New(sha256.New, []byte(g.secret))
	mac.Write([]byte(payload))
	expectedSig := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(token), []byte(expectedSig))
}

func ParseSignedURLParams(token, sub, exp string) (subscriberID string, expires int64, err error) {
	subscriberID = sub
	expires, err = strconv.ParseInt(exp, 10, 64)
	if err != nil {
		return "", 0, fmt.Errorf("invalid expiry: %w", err)
	}
	return subscriberID, expires, nil
}

func ExtractTokenFromQuery(query string) (token, sub, exp string) {
	parts := strings.Split(query, "&")
	for _, part := range parts {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "token":
			token = kv[1]
		case "sub":
			sub = kv[1]
		case "exp":
			exp = kv[1]
		}
	}
	return
}
