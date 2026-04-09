package utils

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func SignURL(contentID, subscriberID, secret string, ttl time.Duration) (string, error) {
	expiresAt := time.Now().Add(ttl).Unix()
	data := fmt.Sprintf("%s:%s:%d", contentID, subscriberID, expiresAt)

	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(data))
	signature := base64.URLEncoding.EncodeToString(h.Sum(nil))

	token := fmt.Sprintf("%s:%d:%s:%s", signature, expiresAt, contentID, subscriberID)
	return token, nil
}

func VerifySignedURL(token, secret string) (contentID, subscriberID string, err error) {
	parts := strings.Split(token, ":")
	if len(parts) != 4 {
		return "", "", fmt.Errorf("invalid token format")
	}

	signature := parts[0]
	expiresStr := parts[1]
	contentID = parts[2]
	subscriberID = parts[3]

	expiresAt, err := strconv.ParseInt(expiresStr, 10, 64)
	if err != nil {
		return "", "", fmt.Errorf("invalid expiration")
	}

	if time.Now().Unix() > expiresAt {
		return "", "", fmt.Errorf("token expired")
	}

	data := fmt.Sprintf("%s:%s:%s", contentID, subscriberID, expiresStr)
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(data))
	expected := base64.URLEncoding.EncodeToString(h.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expected)) {
		return "", "", fmt.Errorf("invalid signature")
	}

	return contentID, subscriberID, nil
}
