package security

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/utils"
	"github.com/stretchr/testify/assert"
)

func TestCredentialRedaction_AllLevels(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		exclude string
	}{
		{
			name:    "github PAT",
			input:   "token=***
			exclude: "ghp_***",
		},
		{
			name:    "bearer token",
			input:   "Authorization: Bearer ***",
			exclude: "abc123def456",
		},
		{
			name:    "gitlab PAT",
			input:   "token=***
			exclude: "glpat_***",
		},
		{
			name:    "password field",
			input:   "password=***
			exclude: "mysecretpass",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			redacted := utils.RedactString(tt.input)
			assert.NotContains(t, redacted, tt.exclude, "credentials should be redacted from: %s", tt.name)
		})
	}
}

func TestCredentialRedaction_NoFalsePositives(t *testing.T) {
	input := "Repository sync completed for github.com/owner/repo"
	redacted := utils.RedactString(input)
	assert.Equal(t, input, redacted, "normal text should not be modified")
}

func TestCredentialRedaction_URL(t *testing.T) {
	input := "https://api.github.com/repos/owner/repo?access_token=***&other=param"
	redacted := utils.RedactURL(input)
	assert.NotContains(t, redacted, "secret", "query parameters should be redacted")
	assert.Equal(t, "https://api.github.com/repos/owner/repo?***", redacted)
}

func TestCredentialRedaction_LogOutput(t *testing.T) {
	// Capture log output
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Log a message containing a secret - redact it before logging
	secret := "ghp_***"
	redactedSecret := utils.RedactString(secret)
	logger.Info("processing repository", "token", redactedSecret)

	output := buf.String()
	assert.NotContains(t, output, secret, "secret should not appear in log output")
	// Verify redaction occurred (should contain asterisks)
	assert.Contains(t, output, "***", "redacted token should appear as asterisks")
}

func TestCredentialRedaction_ErrorMessages(t *testing.T) {
	// Simulate an error message that includes a secret
	secret := "glpat_***"
	errMsg := "authentication failed: token=*** + secret
	redacted := utils.RedactString(errMsg)
	assert.NotContains(t, redacted, secret, "secret should be redacted from error message")
	assert.Contains(t, redacted, "***", "redacted token should appear as asterisks")
}

func TestCredentialRedaction_LogLevels(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	secret := "password=***
	redactedSecret := utils.RedactString(secret)
	levels := []struct {
		name  string
		logFn func(msg string, args ...any)
	}{
		{"debug", logger.Debug},
		{"info", logger.Info},
		{"warn", logger.Warn},
		{"error", logger.Error},
	}
	for _, level := range levels {
		buf.Reset()
		level.logFn("test message", "cred", redactedSecret)
		output := buf.String()
		assert.NotContains(t, output, "superSecret123", "secret should not appear in %s log", level.name)
	}
}

// Note: TRACE level is not currently implemented in slog; if a custom TRACE level
// is added later, this test should be extended.
