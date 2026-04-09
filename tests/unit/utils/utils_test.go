package utils_test

import (
	"testing"

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
		{"scp already", "git@github.com:owner/repo.git", "git@github.com:owner/repo.git"},
		{"ssh protocol", "ssh://git@github.com/owner/repo", "git@github.com:owner/repo.git"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, utils.NormalizeToSSH(tt.input))
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

func TestRedactString(t *testing.T) {
	input := "token=***
	redacted := utils.RedactString(input)
	assert.NotContains(t, redacted, "ghp_***")
}

func TestRedactURL(t *testing.T) {
	redacted := utils.RedactURL("https://api.example.com/data?token=***
	assert.Equal(t, "https://api.example.com/data?***", redacted)
}

func TestRedactURL_NoQuery(t *testing.T) {
	redacted := utils.RedactURL("https://api.example.com/data")
	assert.Equal(t, "https://api.example.com/data", redacted)
}
