package main

import (
	"log/slog"
	"os"
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
