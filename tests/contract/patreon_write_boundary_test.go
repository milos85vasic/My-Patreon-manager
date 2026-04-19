package contract_test

import (
	"os/exec"
	"strings"
	"testing"
)

// TestPatreonWriteBoundary enforces that mutating Patreon calls live only
// in well-known paths. Any new file that starts calling CreatePost or
// UpdatePost is flagged here as a boundary violation.
//
// ALLOWLIST:
//
//	Canonical: process.Publisher is the sole path to Patreon writes.
//	  - internal/services/process/publish.go + its test
//	  - cmd/cli/publish.go + its test (thin adapter + CLI entry)
//	  - internal/providers/patreon/ — the provider implementation itself
//	  - tests/unit/providers/patreon/ — unit tests of the provider
//	  - tests/contract/patreon_write_boundary_test.go — this file
//
//	Test fixtures: mocks that happen to expose CreatePost/UpdatePost.
//	  - tests/mocks/ — mock implementations
func TestPatreonWriteBoundary(t *testing.T) {
	// Resolve repo root so the grep runs from a stable directory regardless
	// of the test's cwd.
	root, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("git rev-parse: %v", err)
	}
	repoRoot := strings.TrimSpace(string(root))

	cmd := exec.Command("git", "grep", "-l", "-E", `\.(CreatePost|UpdatePost)\(`, "--", "*.go")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			// exit 1 = no matches — contract trivially satisfied.
			return
		}
		t.Fatalf("git grep: %v", err)
	}

	// Allowlist prefixes (path relative to repo root).
	allowed := []string{
		// Canonical path — process command's publisher
		"internal/services/process/publish.go",
		"internal/services/process/publish_test.go",

		// Thin CLI entrypoint + adapter
		"cmd/cli/publish.go",
		"cmd/cli/publish_test.go",

		// Provider implementation + its unit tests
		"internal/providers/patreon/",
		"tests/unit/providers/patreon/",

		// This file (may reference the method names in comments/strings)
		"tests/contract/patreon_write_boundary_test.go",

		// Test fixtures
		"tests/mocks/",

		// E2E and chaos tests exercise the real publisher against a fake
		// Patreon server via patreon.Client.CreatePost / UpdatePost. They
		// are downstream consumers of the canonical publish path, not a
		// parallel write route — the fake server is the test boundary.
		"tests/e2e/",
		"tests/chaos/",
	}

	for _, f := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if f == "" {
			continue
		}
		ok := false
		for _, prefix := range allowed {
			if strings.HasPrefix(f, prefix) {
				ok = true
				break
			}
		}
		if !ok {
			t.Errorf("patreon mutation reference outside allowed paths: %s", f)
		}
	}
}
