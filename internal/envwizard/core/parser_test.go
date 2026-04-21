package core_test

import (
	"os"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/envwizard/core"
	"github.com/stretchr/testify/assert"
)

func TestParseEnvFile_Basic(t *testing.T) {
	content := "PORT=8080\n# comment\nADMIN_KEY=secret123\nOPTIONAL_VAR=\n"
	vars, err := core.ParseEnvFile(content)
	assert.NoError(t, err)
	assert.Equal(t, "8080", vars["PORT"])
	assert.Equal(t, "secret123", vars["ADMIN_KEY"])
	assert.Equal(t, "", vars["OPTIONAL_VAR"])
}

func TestParseEnvFile_SkipsCommentsAndBlanks(t *testing.T) {
	content := "# top comment\n\n  \nPORT=8080\n"
	vars, err := core.ParseEnvFile(content)
	assert.NoError(t, err)
	assert.Len(t, vars, 1)
	assert.Equal(t, "8080", vars["PORT"])
}

func TestParseEnvFile_QuotedValues(t *testing.T) {
	content := "KEY=\"quoted value\"\nSINGLE='single quoted'\n"
	vars, err := core.ParseEnvFile(content)
	assert.NoError(t, err)
	assert.Equal(t, `"quoted value"`, vars["KEY"])
	assert.Equal(t, `'single quoted'`, vars["SINGLE"])
}

func TestParseEnvFile_EmptyInput(t *testing.T) {
	vars, err := core.ParseEnvFile("")
	assert.NoError(t, err)
	assert.Empty(t, vars)
}

func TestLoadEnvFile_FromPath(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test*.env")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	tmpFile.WriteString("PORT=9000\nADMIN_KEY=test123\n")
	tmpFile.Close()

	vars, err := core.LoadEnvFile(tmpFile.Name())
	assert.NoError(t, err)
	assert.Equal(t, "9000", vars["PORT"])
	assert.Equal(t, "test123", vars["ADMIN_KEY"])
}

func TestLoadEnvFile_NotFound(t *testing.T) {
	_, err := core.LoadEnvFile("/nonexistent/.env")
	assert.Error(t, err)
}

func TestGenerateEnvFile_Basic(t *testing.T) {
	vars := map[string]string{"PORT": "3000", "DEBUG": "true"}
	content := core.GenerateEnvFile(vars)
	assert.Contains(t, content, "PORT=3000")
	assert.Contains(t, content, "DEBUG=true")
}

func TestRoundTrip(t *testing.T) {
	original := map[string]string{"PORT": "5000", "DEBUG": "true"}
	content := core.GenerateEnvFile(original)

	loaded, err := core.ParseEnvFile(content)
	assert.NoError(t, err)
	assert.Equal(t, original["PORT"], loaded["PORT"])
	assert.Equal(t, original["DEBUG"], loaded["DEBUG"])
}
