package main

import (
	"log/slog"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"error", slog.LevelError},
		{"unknown", slog.LevelInfo},
		{"", slog.LevelInfo},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			level := parseLogLevel(tt.input)
			assert.Equal(t, tt.expected, level)
		})
	}
}

func TestSetupProviders_Empty(t *testing.T) {
	cfg := &config.Config{}
	providers := setupProviders(cfg)
	assert.Empty(t, providers)
}

func TestSetupProviders_GitHub(t *testing.T) {
	cfg := &config.Config{
		GitHubToken: "test-token",
	}
	providers := setupProviders(cfg)
	assert.Len(t, providers, 1)
}

func TestSetupProviders_GitLab(t *testing.T) {
	cfg := &config.Config{
		GitLabToken: "test-token",
	}
	providers := setupProviders(cfg)
	assert.Len(t, providers, 1)
}

func TestSetupProviders_GitFlic(t *testing.T) {
	cfg := &config.Config{
		GitFlicToken: "test-token",
	}
	providers := setupProviders(cfg)
	assert.Len(t, providers, 1)
}

func TestSetupProviders_GitVerse(t *testing.T) {
	cfg := &config.Config{
		GitVerseToken: "test-token",
	}
	providers := setupProviders(cfg)
	assert.Len(t, providers, 1)
}

func TestSetupProviders_Multiple(t *testing.T) {
	cfg := &config.Config{
		GitHubToken:   "gh",
		GitLabToken:   "gl",
		GitFlicToken:  "gf",
		GitVerseToken: "gv",
	}
	providers := setupProviders(cfg)
	assert.Len(t, providers, 4)
}
