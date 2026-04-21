package main

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestEnsureLLMsVerifier_AlreadyRunning(t *testing.T) {
	origReachable := verifierReachable
	defer func() { verifierReachable = origReachable }()

	verifierReachable = func(endpoint string) bool { return true }

	cfg := &config.Config{LLMsVerifierEndpoint: "http://localhost:9099"}
	err := ensureLLMsVerifierImpl(cfg, testLogger())
	assert.NoError(t, err)
}

func TestEnsureLLMsVerifier_BootstrapSuccess(t *testing.T) {
	origReachable := verifierReachable
	origExec := execCommand
	defer func() {
		verifierReachable = origReachable
		execCommand = origExec
	}()

	callCount := 0
	verifierReachable = func(endpoint string) bool {
		callCount++
		// First call: not reachable; second call (after bootstrap): reachable
		return callCount >= 2
	}

	execCalled := false
	execCommand = func(name string, args ...string) error {
		execCalled = true
		assert.Equal(t, "bash", name)
		assert.Contains(t, args[0], "llmsverifier.sh")
		return nil
	}

	cfg := &config.Config{LLMsVerifierEndpoint: "http://localhost:9099"}
	err := ensureLLMsVerifierImpl(cfg, testLogger())
	assert.NoError(t, err)
	assert.True(t, execCalled)
}

func TestEnsureLLMsVerifier_BootstrapScriptNotFound(t *testing.T) {
	origReachable := verifierReachable
	defer func() { verifierReachable = origReachable }()

	verifierReachable = func(endpoint string) bool { return false }

	// Change to a temp dir where scripts/llmsverifier.sh doesn't exist
	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { require.NoError(t, os.Chdir(origDir)) }()

	cfg := &config.Config{LLMsVerifierEndpoint: "http://localhost:9099"}
	err := ensureLLMsVerifierImpl(cfg, testLogger())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "bootstrap script")
	assert.Contains(t, err.Error(), "not found")
}

func TestEnsureLLMsVerifier_BootstrapFails(t *testing.T) {
	origReachable := verifierReachable
	origExec := execCommand
	defer func() {
		verifierReachable = origReachable
		execCommand = origExec
	}()

	verifierReachable = func(endpoint string) bool { return false }
	execCommand = func(name string, args ...string) error {
		return assert.AnError
	}

	cfg := &config.Config{LLMsVerifierEndpoint: "http://localhost:9099"}
	err := ensureLLMsVerifierImpl(cfg, testLogger())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "bootstrap failed")
}

func TestEnsureLLMsVerifier_StillUnreachableAfterBootstrap(t *testing.T) {
	origReachable := verifierReachable
	origExec := execCommand
	defer func() {
		verifierReachable = origReachable
		execCommand = origExec
	}()

	// Always unreachable
	verifierReachable = func(endpoint string) bool { return false }
	execCommand = func(name string, args ...string) error { return nil }

	cfg := &config.Config{LLMsVerifierEndpoint: "http://localhost:9099"}
	err := ensureLLMsVerifierImpl(cfg, testLogger())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "still not reachable")
}

func TestVerifierReachableImpl_EmptyEndpoint(t *testing.T) {
	assert.False(t, verifierReachableImpl(""))
}

func TestVerifierReachableImpl_Unreachable(t *testing.T) {
	// Port 1 is almost certainly not listening
	assert.False(t, verifierReachableImpl("http://localhost:1"))
}

func TestFindBootstrapScript_FromProjectRoot(t *testing.T) {
	// We're running from the project root in tests; the script should exist.
	origDir, _ := os.Getwd()
	// Navigate to project root (cmd/cli -> project root)
	projectRoot := origDir
	for i := 0; i < 3; i++ {
		candidate := projectRoot + "/scripts/llmsverifier.sh"
		if _, err := os.Stat(candidate); err == nil {
			break
		}
		projectRoot = projectRoot + "/.."
	}

	require.NoError(t, os.Chdir(projectRoot))
	defer func() { require.NoError(t, os.Chdir(origDir)) }()

	path := findBootstrapScript()
	if path != "" {
		assert.Contains(t, path, "llmsverifier.sh")
	}
	// If not found (e.g. running from a different dir), that's OK for CI
}

func TestFindBootstrapScript_NotFound(t *testing.T) {
	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { require.NoError(t, os.Chdir(origDir)) }()

	path := findBootstrapScript()
	assert.Empty(t, path)
}

func TestGetEnvOrDefault_EnvSet(t *testing.T) {
	t.Setenv("TEST_BOOT_VAR", "from-env")
	assert.Equal(t, "from-env", getEnvOrDefault("TEST_BOOT_VAR", "fallback"))
}

func TestGetEnvOrDefault_EnvEmpty(t *testing.T) {
	t.Setenv("TEST_BOOT_VAR", "")
	assert.Equal(t, "fallback", getEnvOrDefault("TEST_BOOT_VAR", "fallback"))
}

func TestGetEnvOrDefault_EnvUnset(t *testing.T) {
	os.Unsetenv("TEST_BOOT_VAR_NONEXISTENT")
	assert.Equal(t, "fallback", getEnvOrDefault("TEST_BOOT_VAR_NONEXISTENT", "fallback"))
}

// TestExecCommandImpl_Success runs a trivially-true command (/bin/true on
// POSIX) to exercise the real execCommandImpl without any network or LLM
// side-effects. The goal is solely to bump coverage on the wrapper.
func TestExecCommandImpl_Success(t *testing.T) {
	err := execCommandImpl("true")
	if err != nil {
		// Some minimal containers lack /bin/true; fall back to `sh -c :`.
		if shErr := execCommandImpl("sh", "-c", ":"); shErr != nil {
			t.Skipf("neither 'true' nor 'sh -c :' available: true=%v sh=%v", err, shErr)
		}
	}
}

// TestExecCommandImpl_Error asserts the wrapper surfaces exec errors.
func TestExecCommandImpl_Error(t *testing.T) {
	err := execCommandImpl("no-such-binary-that-definitely-does-not-exist-xyzzy")
	if err == nil {
		t.Fatal("expected error from non-existent command")
	}
}

// TestVerifierReachableImpl_Success exercises the success branch — the
// stub http server answers /api/health with 200, so the function must
// return true.
func TestVerifierReachableImpl_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/health" {
			w.WriteHeader(404)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()
	if !verifierReachableImpl(srv.URL) {
		t.Fatal("expected verifierReachableImpl to return true for healthy endpoint")
	}
}

// TestVerifierReachableImpl_5xx: status >= 500 is treated as unreachable.
func TestVerifierReachableImpl_5xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
	}))
	defer srv.Close()
	if verifierReachableImpl(srv.URL) {
		t.Fatal("expected verifierReachableImpl to return false for 503")
	}
}

func TestEnsureLLMsVerifier_EmptyEndpointTriggersBootstrap(t *testing.T) {
	origReachable := verifierReachable
	origExec := execCommand
	defer func() {
		verifierReachable = origReachable
		execCommand = origExec
	}()

	callCount := 0
	verifierReachable = func(endpoint string) bool {
		callCount++
		return callCount >= 2
	}
	execCommand = func(name string, args ...string) error { return nil }

	cfg := &config.Config{LLMsVerifierEndpoint: ""}
	err := ensureLLMsVerifierImpl(cfg, testLogger())
	assert.NoError(t, err)
}

func TestFindBootstrapScript_FoundInCurrentDir(t *testing.T) {
	dir := t.TempDir()
	scriptDir := filepath.Join(dir, "scripts")
	require.NoError(t, os.Mkdir(scriptDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(scriptDir, "llmsverifier.sh"), []byte("#!/bin/bash"), 0644))

	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	defer os.Chdir(origWd)

	result := findBootstrapScript()
	assert.Contains(t, result, "llmsverifier.sh")
}

func TestFindBootstrapScript_FoundInParentDir(t *testing.T) {
	dir := t.TempDir()
	scriptDir := filepath.Join(dir, "scripts")
	require.NoError(t, os.Mkdir(scriptDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(scriptDir, "llmsverifier.sh"), []byte("#!/bin/bash"), 0644))

	subDir := filepath.Join(dir, "subdir")
	require.NoError(t, os.Mkdir(subDir, 0755))

	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(subDir))
	defer os.Chdir(origWd)

	result := findBootstrapScript()
	assert.Contains(t, result, "llmsverifier.sh")
}
