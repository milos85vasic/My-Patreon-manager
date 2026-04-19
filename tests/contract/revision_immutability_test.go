package contract_test

import (
	"os/exec"
	"strings"
	"testing"
)

// TestRevisionBodyImmutability enforces that nothing in the project
// issues a SQL UPDATE that touches a content_revisions column which is
// supposed to be immutable (body, title, fingerprint). Only status plus
// the published-marker columns (patreon_post_id, published_to_patreon_at)
// may be UPDATEd after insert.
//
// This grep is deliberately conservative. Multi-line UPDATE … SET body=?
// statements are matched via the multiline pattern. Runs over *.go and
// *.sql files only; comments in .md docs are out of scope.
func TestRevisionBodyImmutability(t *testing.T) {
	root, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("git rev-parse: %v", err)
	}
	repoRoot := strings.TrimSpace(string(root))

	// Pattern: `UPDATE content_revisions ... SET ... (body|title|fingerprint)`
	// Allow whitespace and column-list variations between UPDATE and SET.
	// Use git grep's PCRE mode (-P) for multi-line tolerance.
	cmd := exec.Command("git", "grep", "-l", "-P",
		`(?s)UPDATE\s+content_revisions.*?SET\s.*?\b(body|title|fingerprint)\b\s*=`,
		"--", "*.go", "*.sql")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return // no matches; contract satisfied
		}
		t.Fatalf("git grep: %v", err)
	}
	hits := strings.TrimSpace(string(out))
	if hits == "" {
		return
	}
	// Allowlist — the contract test itself may reference these column
	// names in its comments.
	allowed := map[string]bool{
		"tests/contract/revision_immutability_test.go": true,
	}
	for _, f := range strings.Split(hits, "\n") {
		if allowed[f] {
			continue
		}
		t.Errorf("forbidden UPDATE on content_revisions immutable columns: %s", f)
	}
}
