package utils_test

import (
	"strings"
	"testing"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/utils"
	"github.com/stretchr/testify/assert"
)

func TestNewUUID(t *testing.T) {
	id := utils.NewUUID()
	assert.NotEmpty(t, id)
	assert.Len(t, id, 36)

	id2 := utils.NewUUID()
	assert.NotEqual(t, id, id2)
}

func TestContentHash(t *testing.T) {
	h1 := utils.ContentHash("hello")
	h2 := utils.ContentHash("hello")
	h3 := utils.ContentHash("world")

	assert.Equal(t, h1, h2)
	assert.NotEqual(t, h1, h3)
	assert.Len(t, h1, 64)
}

func TestREADMEHash(t *testing.T) {
	h := utils.READMEHash("# README")
	assert.Len(t, h, 64)
}

func TestNormalizeToSSH(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"https", "https://github.com/owner/repo", "git@github.com:owner/repo.git"},
		{"https with .git", "https://github.com/owner/repo.git", "git@github.com:owner/repo.git"},
		{"scp already with .git", "git@github.com:owner/repo.git", "git@github.com:owner/repo.git"},
		{"scp already without .git", "git@github.com:owner/repo", "git@github.com:owner/repo"},
		{"malformed scp no slash", "git@github.com:owner", "git@github.com:owner"},
		{"ssh protocol", "ssh://git@github.com/owner/repo", "git@github.com:owner/repo.git"},
		{"scp with extra colon", "git@github.com:owner:repo", "git@github.com:owner:repo"},
		{"ssh protocol with .git", "ssh://git@github.com/owner/repo.git", "git@github.com:owner/repo.git"},
		{"invalid URL", "foobar", "foobar"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, utils.NormalizeToSSH(tt.input))
		})
	}
}

func TestNormalizeHTTPS(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"scp", "git@github.com:owner/repo.git", "https://github.com/owner/repo.git"},
		{"ssh protocol", "ssh://git@github.com/owner/repo", "https://github.com/owner/repo.git"},
		{"already https", "https://github.com/owner/repo.git", "https://github.com/owner/repo.git"},
		{"no .git suffix", "git@github.com:owner/repo", "https://github.com/owner/repo.git"},
		{"invalid url", "invalid", "invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, utils.NormalizeHTTPS(tt.input))
		})
	}
}

func TestJaccardSimilarity(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want float64
	}{
		{"identical", "hello world", "hello world", 1.0},
		{"no overlap", "hello world", "foo bar", 0.0},
		{"partial", "hello world", "hello there", 0.3333333333333333}, // intersect 1, union 3
		{"empty a", "", "hello", 0.0},
		{"empty b", "hello", "", 0.0},
		{"punctuation", "hello, world!", "hello world", 1.0},
		{"case insensitive", "HELLO WORLD", "hello world", 1.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.InDelta(t, tt.want, utils.JaccardSimilarity(tt.a, tt.b), 0.0001)
		})
	}
}

func TestToJSON(t *testing.T) {
	result, err := utils.ToJSON(map[string]string{"key": "value"})
	assert.NoError(t, err)
	assert.Contains(t, result, "key")
	assert.Contains(t, result, "value")
}

func TestFromJSON(t *testing.T) {
	var m map[string]string
	err := utils.FromJSON(`{"key":"value"}`, &m)
	assert.NoError(t, err)
	assert.Equal(t, "value", m["key"])
}

func TestFromJSON_Empty(t *testing.T) {
	var m map[string]string
	err := utils.FromJSON("", &m)
	assert.NoError(t, err)
}

func TestFromJSON_Invalid(t *testing.T) {
	var m map[string]string
	err := utils.FromJSON("{invalid json", &m)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "fromJSON:")
}

func TestToJSON_Error(t *testing.T) {
	// Channel cannot be marshaled to JSON
	ch := make(chan int)
	_, err := utils.ToJSON(ch)
	assert.Error(t, err)
}

func TestRedactString(t *testing.T) {
	input := "token=ghp_***"
	redacted := utils.RedactString(input)
	assert.NotContains(t, redacted, "ghp_***")
}

func TestRedactURL(t *testing.T) {
	redacted := utils.RedactURL("https://api.example.com/data?token=***")
	assert.Equal(t, "https://api.example.com/data?***", redacted)
}

func TestRedactURL_NoQuery(t *testing.T) {
	redacted := utils.RedactURL("https://api.example.com/data")
	assert.Equal(t, "https://api.example.com/data", redacted)
}

func TestSignURL(t *testing.T) {
	token, err := utils.SignURL("content123", "user456", "secret", time.Minute*5)
	assert.NoError(t, err)
	assert.NotEmpty(t, token)

	parts := strings.Split(token, ":")
	assert.Len(t, parts, 4)
}

func TestVerifySignedURL_Valid(t *testing.T) {
	token, err := utils.SignURL("content123", "user456", "secret", time.Minute*5)
	assert.NoError(t, err)

	contentID, subscriberID, err := utils.VerifySignedURL(token, "secret")
	assert.NoError(t, err)
	assert.Equal(t, "content123", contentID)
	assert.Equal(t, "user456", subscriberID)
}

func TestVerifySignedURL_InvalidSignature(t *testing.T) {
	token, err := utils.SignURL("content123", "user456", "secret", time.Minute*5)
	assert.NoError(t, err)

	_, _, err = utils.VerifySignedURL(token, "wrong-secret")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid signature")
}

func TestVerifySignedURL_Expired(t *testing.T) {
	token, err := utils.SignURL("content123", "user456", "secret", -time.Minute*5)
	assert.NoError(t, err)

	_, _, err = utils.VerifySignedURL(token, "secret")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "token expired")
}

func TestVerifySignedURL_InvalidFormat(t *testing.T) {
	_, _, err := utils.VerifySignedURL("invalid", "secret")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid token format")
}

func TestVerifySignedURL_InvalidExpiration(t *testing.T) {
	token := "signature:notanumber:content:user"
	_, _, err := utils.VerifySignedURL(token, "secret")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid expiration")
}
