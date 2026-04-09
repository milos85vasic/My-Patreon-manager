package utils

import (
	"crypto/sha256"
	"encoding/hex"
)

func ContentHash(body string) string {
	h := sha256.New()
	h.Write([]byte(body))
	return hex.EncodeToString(h.Sum(nil))
}

func READMEHash(content string) string {
	h := sha256.New()
	h.Write([]byte(content))
	return hex.EncodeToString(h.Sum(nil))
}
