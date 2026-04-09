package access_test

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/services/access"
	"github.com/stretchr/testify/assert"
)

func parseQuery(url string) (token, sub string, exp int64) {
	idx := strings.Index(url, "?")
	if idx < 0 {
		return
	}
	t, s, e := access.ExtractTokenFromQuery(url[idx+1:])
	exp, _ = strconv.ParseInt(e, 10, 64)
	return t, s, exp
}

func TestSignedURLGenerator_GenerateAndVerify(t *testing.T) {
	gen := access.NewSignedURLGenerator("test-secret", 1*time.Hour)
	url := gen.GenerateSignedURL("content-1", "sub-1")
	assert.Contains(t, url, "/download/content-1")
	assert.Contains(t, url, "token=***
}

func TestSignedURLGenerator_VerifyValid(t *testing.T) {
	gen := access.NewSignedURLGenerator("test-secret", 1*time.Hour)
	url := gen.GenerateSignedURL("content-1", "sub-1")
	token, sub, exp := parseQuery(url)
	valid := gen.VerifySignedURL(token, "content-1", sub, exp)
	assert.True(t, valid)
}

func TestSignedURLGenerator_WrongContentID(t *testing.T) {
	gen := access.NewSignedURLGenerator("test-secret", 1*time.Hour)
	url := gen.GenerateSignedURL("content-1", "sub-1")
	token, sub, exp := parseQuery(url)
	valid := gen.VerifySignedURL(token, "content-2", sub, exp)
	assert.False(t, valid)
}

func TestSignedURLGenerator_WrongSecret(t *testing.T) {
	gen1 := access.NewSignedURLGenerator("secret-1", 1*time.Hour)
	gen2 := access.NewSignedURLGenerator("secret-2", 1*time.Hour)
	url := gen1.GenerateSignedURL("content-1", "sub-1")
	token, sub, exp := parseQuery(url)
	valid := gen2.VerifySignedURL(token, "content-1", sub, exp)
	assert.False(t, valid)
}

func TestSignedURLGenerator_ExpiredToken(t *testing.T) {
	gen := access.NewSignedURLGenerator("test-secret", -1*time.Second)
	url := gen.GenerateSignedURL("content-1", "sub-1")
	token, sub, exp := parseQuery(url)
	valid := gen.VerifySignedURL(token, "content-1", sub, exp)
	assert.False(t, valid)
}

func TestSignedURLGenerator_ForgedToken(t *testing.T) {
	gen := access.NewSignedURLGenerator("test-secret", 1*time.Hour)
	valid := gen.VerifySignedURL("forged-token", "content-1", "sub-1", time.Now().Add(1*time.Hour).Unix())
	assert.False(t, valid)
}

func TestParseSignedURLParams(t *testing.T) {
	sub, exp, err := access.ParseSignedURLParams("token", "sub-1", "1700000000")
	assert.NoError(t, err)
	assert.Equal(t, "sub-1", sub)
	assert.Equal(t, int64(1700000000), exp)
}

func TestParseSignedURLParams_InvalidExp(t *testing.T) {
	_, _, err := access.ParseSignedURLParams("token", "sub-1", "not-a-number")
	assert.Error(t, err)
}

func TestExtractTokenFromQuery(t *testing.T) {
	token, sub, exp := access.ExtractTokenFromQuery("token=***&sub=user1&exp=12345")
	assert.Equal(t, "abc", token)
	assert.Equal(t, "user1", sub)
	assert.Equal(t, "12345", exp)
}
