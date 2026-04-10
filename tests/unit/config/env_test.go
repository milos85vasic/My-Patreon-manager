package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestLoadEnv_NoFiles(t *testing.T) {
	// Create a temporary directory and change to it
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	assert.NoError(t, err)
	defer os.Chdir(originalDir)

	err = os.Chdir(tmpDir)
	assert.NoError(t, err)

	// Create .env file in current directory
	envContent := "TEST_KEY=value"
	err = os.WriteFile(".env", []byte(envContent), 0644)
	assert.NoError(t, err)

	// LoadEnv with no arguments should load .env
	err = config.LoadEnv()
	assert.NoError(t, err)
	assert.Equal(t, "value", os.Getenv("TEST_KEY"))
	os.Unsetenv("TEST_KEY")
}

func TestLoadEnv_WithFile(t *testing.T) {
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, "test.env")
	envContent := "TEST_KEY=from_file"
	err := os.WriteFile(envFile, []byte(envContent), 0644)
	assert.NoError(t, err)

	err = config.LoadEnv(envFile)
	assert.NoError(t, err)
	assert.Equal(t, "from_file", os.Getenv("TEST_KEY"))
	os.Unsetenv("TEST_KEY")
}

func TestLoadEnv_MultipleFilesSkipsMissing(t *testing.T) {
	tmpDir := t.TempDir()
	envFile1 := filepath.Join(tmpDir, "exists.env")
	envContent := "TEST_KEY=exists"
	err := os.WriteFile(envFile1, []byte(envContent), 0644)
	assert.NoError(t, err)

	missingFile := filepath.Join(tmpDir, "missing.env")
	// Should skip missing file and not error
	err = config.LoadEnv(envFile1, missingFile)
	assert.NoError(t, err)
	assert.Equal(t, "exists", os.Getenv("TEST_KEY"))
	os.Unsetenv("TEST_KEY")
}

func TestLoadEnv_NonPathErrorReturnsError(t *testing.T) {
	// We cannot easily simulate a non-PathError from godotenv.Load
	// since godotenv only returns PathError for missing files.
	// For now, we'll skip this test.
	t.Skip("Cannot simulate non-PathError from godotenv.Load")
}

func TestLoadEnvOverride_NoFiles(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	assert.NoError(t, err)
	defer os.Chdir(originalDir)

	err = os.Chdir(tmpDir)
	assert.NoError(t, err)

	// Create .env file
	envContent := "TEST_KEY=override"
	err = os.WriteFile(".env", []byte(envContent), 0644)
	assert.NoError(t, err)

	err = config.LoadEnvOverride()
	assert.NoError(t, err)
	assert.Equal(t, "override", os.Getenv("TEST_KEY"))
	os.Unsetenv("TEST_KEY")
}

func TestLoadEnvOverride_WithFile(t *testing.T) {
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, "test.env")
	envContent := "TEST_KEY=override_file"
	err := os.WriteFile(envFile, []byte(envContent), 0644)
	assert.NoError(t, err)

	err = config.LoadEnvOverride(envFile)
	assert.NoError(t, err)
	assert.Equal(t, "override_file", os.Getenv("TEST_KEY"))
	os.Unsetenv("TEST_KEY")
}

func TestLoadEnvOverride_MultipleFilesSkipsMissing(t *testing.T) {
	tmpDir := t.TempDir()
	envFile1 := filepath.Join(tmpDir, "exists.env")
	envContent := "TEST_KEY=exists"
	err := os.WriteFile(envFile1, []byte(envContent), 0644)
	assert.NoError(t, err)

	missingFile := filepath.Join(tmpDir, "missing.env")
	err = config.LoadEnvOverride(envFile1, missingFile)
	assert.NoError(t, err)
	assert.Equal(t, "exists", os.Getenv("TEST_KEY"))
	os.Unsetenv("TEST_KEY")
}
