package core_test

import (
	"os"
	"strings"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/envwizard/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var genTestVars = []*core.EnvVar{
	{Name: "PORT", Description: "HTTP port", Category: &core.Category{ID: "server", Name: "Server", Order: 1}, Required: true, Default: "8080", Validation: core.ValidationPort},
	{Name: "ADMIN_KEY", Description: "Admin key", Category: &core.Category{ID: "security", Name: "Security", Order: 2}, Required: true, Secret: true},
	{Name: "DEBUG", Description: "Debug mode", Category: &core.Category{ID: "server", Name: "Server", Order: 1}, Required: false, Default: "false", Validation: core.ValidationBoolean},
}

func TestGenerator_ProduceEnvFile(t *testing.T) {
	w := core.NewWizard(genTestVars)
	w.SetValue("PORT", "3000")
	w.SetValue("ADMIN_KEY", "secret123")

	gen := core.NewGenerator(w)
	output := gen.ProduceEnvFile(false)

	assert.Contains(t, output, "PORT=3000")
	assert.Contains(t, output, "ADMIN_KEY=secret123")
	assert.Contains(t, output, "# Server")
	assert.Contains(t, output, "# HTTP port")
}

func TestGenerator_ProduceEnvFile_MaskSecrets(t *testing.T) {
	w := core.NewWizard(genTestVars)
	w.SetValue("ADMIN_KEY", "secret123")

	gen := core.NewGenerator(w)
	output := gen.ProduceEnvFile(true)

	assert.Contains(t, output, "ADMIN_KEY=*********")
	assert.NotContains(t, output, "secret123")
}

func TestGenerator_ProduceEnvFile_SkippedVars(t *testing.T) {
	w := core.NewWizard(genTestVars)
	w.SetValue("PORT", "8080")
	w.Skip("ADMIN_KEY")

	gen := core.NewGenerator(w)
	output := gen.ProduceEnvFile(false)

	assert.Contains(t, output, "PORT=8080")
	assert.NotContains(t, output, "ADMIN_KEY")
}

func TestGenerator_ProduceEnvFile_UsesDefaults(t *testing.T) {
	w := core.NewWizard(genTestVars)

	gen := core.NewGenerator(w)
	output := gen.ProduceEnvFile(false)

	assert.Contains(t, output, "PORT=8080")
	assert.Contains(t, output, "DEBUG=false")
}

func TestGenerator_SaveToPath(t *testing.T) {
	w := core.NewWizard(genTestVars)
	w.SetValue("PORT", "3000")

	tmpFile, err := os.CreateTemp("", "test*.env")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	gen := core.NewGenerator(w)
	err = gen.SaveToPath(tmpFile.Name(), false)
	require.NoError(t, err)

	data, err := os.ReadFile(tmpFile.Name())
	require.NoError(t, err)
	assert.Contains(t, string(data), "PORT=3000")
}

func TestGenerator_SaveToPath_FilePermissions(t *testing.T) {
	w := core.NewWizard(genTestVars)
	w.SetValue("PORT", "8080")

	tmpFile, err := os.CreateTemp("", "test*.env")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	gen := core.NewGenerator(w)
	require.NoError(t, gen.SaveToPath(tmpFile.Name(), false))

	info, err := os.Stat(tmpFile.Name())
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestGenerator_CategoriesGrouped(t *testing.T) {
	w := core.NewWizard(genTestVars)
	w.SetValue("PORT", "8080")
	w.SetValue("ADMIN_KEY", "key")

	gen := core.NewGenerator(w)
	output := gen.ProduceEnvFile(false)

	serverIdx := strings.Index(output, "# Server")
	securityIdx := strings.Index(output, "# Security")
	assert.True(t, serverIdx < securityIdx, "Server should come before Security")
}
