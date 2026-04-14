package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/config"
)

// ensureLLMsVerifier checks whether the LLMsVerifier service is reachable.
// If it is not, the function attempts to start it by running the bootstrap
// script (scripts/llmsverifier.sh) and waits for it to become healthy.
// On success the function reloads the .env so that LLMSVERIFIER_ENDPOINT
// and LLMSVERIFIER_API_KEY pick up any values the script wrote.
//
// The function is a package-level variable so tests can replace it.
var ensureLLMsVerifier = ensureLLMsVerifierImpl

// verifierReachable is separated so tests can override network checks.
var verifierReachable = verifierReachableImpl

// execCommand is separated so tests can override subprocess execution.
var execCommand = execCommandImpl

func verifierReachableImpl(endpoint string) bool {
	if endpoint == "" {
		return false
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(endpoint + "/v1/models")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 500
}

func execCommandImpl(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func ensureLLMsVerifierImpl(cfg *config.Config, logger *slog.Logger) error {
	// If already reachable, nothing to do.
	if verifierReachable(cfg.LLMsVerifierEndpoint) {
		logger.Debug("LLMsVerifier is already running",
			slog.String("endpoint", cfg.LLMsVerifierEndpoint))
		return nil
	}

	logger.Info("LLMsVerifier not reachable, attempting auto-start...")

	// Locate the bootstrap script relative to the working directory.
	scriptPath := findBootstrapScript()
	if scriptPath == "" {
		return fmt.Errorf(
			"LLMsVerifier is not reachable at %q and the bootstrap script "+
				"(scripts/llmsverifier.sh) was not found.\n"+
				"Either start the service manually or run from the project root",
			cfg.LLMsVerifierEndpoint)
	}

	logger.Info("running bootstrap script", slog.String("script", scriptPath))
	if err := execCommand("bash", scriptPath); err != nil {
		return fmt.Errorf("LLMsVerifier bootstrap failed: %w", err)
	}

	// The script updates .env with fresh LLMSVERIFIER_ENDPOINT and
	// LLMSVERIFIER_API_KEY. Reload so the current process picks them up.
	_ = config.LoadEnvOverride(".env")
	cfg.LLMsVerifierEndpoint = getEnvOrDefault("LLMSVERIFIER_ENDPOINT", cfg.LLMsVerifierEndpoint)
	cfg.LLMsVerifierAPIKey = getEnvOrDefault("LLMSVERIFIER_API_KEY", cfg.LLMsVerifierAPIKey)

	// Final reachability check after bootstrap.
	if !verifierReachable(cfg.LLMsVerifierEndpoint) {
		return fmt.Errorf(
			"LLMsVerifier bootstrap script completed but service is still "+
				"not reachable at %q", cfg.LLMsVerifierEndpoint)
	}

	logger.Info("LLMsVerifier auto-started successfully",
		slog.String("endpoint", cfg.LLMsVerifierEndpoint))
	return nil
}

// findBootstrapScript looks for scripts/llmsverifier.sh starting from the
// working directory and walking up (max 3 levels). Returns the path if
// found, or empty string.
func findBootstrapScript() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for i := 0; i < 4; i++ {
		candidate := filepath.Join(dir, "scripts", "llmsverifier.sh")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

func getEnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
