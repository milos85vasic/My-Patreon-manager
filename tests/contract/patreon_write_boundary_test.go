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
// ALLOWLIST — three tiers:
//
//	Canonical (permanent): the new process.Publisher is the sole
//	intended future path to Patreon writes.
//	  - internal/services/process/publish.go + its test
//	  - cmd/cli/publish.go + its test (thin adapter + CLI entry)
//	  - internal/providers/patreon/ — the provider implementation itself
//	  - tests/unit/providers/patreon/ — unit tests of the provider
//	  - tests/contract/patreon_write_boundary_test.go — this file
//
//	Transitional (to be retired): the legacy sync orchestrator still
//	issues Patreon writes from Orchestrator.Run and Orchestrator.PublishOnly.
//	These should be removed once the process command fully supersedes sync.
//	  - internal/services/sync/orchestrator.go
//	  - tests for the legacy orchestrator (orchestrator_*_test.go)
//
//	Test fixtures: the e2e test exercises the legacy sync path and
//	uses a mock patreon client that happens to expose CreatePost/UpdatePost.
//	  - tests/e2e/full_sync_test.go
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

		// TRANSITIONAL — legacy sync path, scheduled for retirement once
		// the process command fully supersedes it.
		// TODO(process-migration): remove once Orchestrator.Run and
		// Orchestrator.PublishOnly no longer issue Patreon writes.
		"internal/services/sync/orchestrator.go",
		"internal/services/sync/orchestrator_split_test.go",
		"internal/services/sync/orchestrator_coverage_test.go",

		// Test fixtures
		// TODO(process-migration): retire alongside the legacy sync path.
		"tests/e2e/full_sync_test.go",
		"tests/mocks/",
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
