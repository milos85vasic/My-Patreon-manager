package security

import (
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
