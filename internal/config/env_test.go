package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnv_NoArgs_MissingFile(t *testing.T) {
	// When called with no args, godotenv.Load looks for .env in the current
	// directory. We change to a temp directory where no .env exists.
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(orig)

	// godotenv.Load returns an error when .env is missing
	err = LoadEnv()
	if err == nil {
		t.Fatal("expected error when .env is missing")
	}
}

func TestLoadEnv_WithFiles_FileNotFound(t *testing.T) {
	// PathError for a missing file is silently skipped
	err := LoadEnv("/nonexistent/.env")
	if err != nil {
		t.Fatalf("expected no error for missing file (PathError): got %v", err)
	}
}

func TestLoadEnv_WithFiles_Success(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	if err := os.WriteFile(envFile, []byte("TEST_LOAD_ENV_VAR=hello\n"), 0644); err != nil {
		t.Fatal(err)
	}
	err := LoadEnv(envFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Verify the env var was loaded
	if v := os.Getenv("TEST_LOAD_ENV_VAR"); v != "hello" {
		t.Fatalf("expected TEST_LOAD_ENV_VAR=hello, got %q", v)
	}
}

func TestLoadEnv_WithFiles_ParseError(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	// Write a file with invalid content (unclosed quote triggers parse error)
	if err := os.WriteFile(envFile, []byte("KEY=\"unclosed\n"), 0644); err != nil {
		t.Fatal(err)
	}
	err := LoadEnv(envFile)
	if err == nil {
		t.Fatal("expected parse error for invalid .env file")
	}
}

func TestLoadEnvOverride_NoArgs_MissingFile(t *testing.T) {
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(orig)

	err = LoadEnvOverride()
	if err == nil {
		t.Fatal("expected error when .env is missing")
	}
}

func TestLoadEnvOverride_WithFiles_FileNotFound(t *testing.T) {
	err := LoadEnvOverride("/nonexistent/.env")
	if err != nil {
		t.Fatalf("expected no error for missing file (PathError): got %v", err)
	}
}

func TestLoadEnvOverride_WithFiles_Success(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env.override")
	t.Setenv("TEST_OVERRIDE_VAR", "original")
	if err := os.WriteFile(envFile, []byte("TEST_OVERRIDE_VAR=overridden\n"), 0644); err != nil {
		t.Fatal(err)
	}
	err := LoadEnvOverride(envFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v := os.Getenv("TEST_OVERRIDE_VAR"); v != "overridden" {
		t.Fatalf("expected TEST_OVERRIDE_VAR=overridden, got %q", v)
	}
}

func TestLoadEnvOverride_WithFiles_ParseError(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	if err := os.WriteFile(envFile, []byte("KEY=\"unclosed\n"), 0644); err != nil {
		t.Fatal(err)
	}
	err := LoadEnvOverride(envFile)
	if err == nil {
		t.Fatal("expected parse error for invalid .env file")
	}
}
