// Package process implements the versioned content pipeline for the
// patreon-manager process command: per-repo pipeline transactions,
// fingerprint-based deduplication, drift detection, and single-runner
// locking via the process_runs table.
//
// See docs/superpowers/specs/2026-04-18-process-command-design.md for
// the design contract and docs/superpowers/plans/2026-04-18-process-command.md
// for the implementation plan.
package process

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

var multiWhitespace = regexp.MustCompile(`\s+`)

// Fingerprint returns a stable sha256 hex of the normalized body plus the
// illustration content hash. Whitespace differences in the body do not
// change the fingerprint so re-rendered Patreon content (which may have
// different whitespace) does not trigger false drift. A null byte
// separator prevents concat-ambiguity collisions between body and
// illustration inputs.
func Fingerprint(body, illustrationHash string) string {
	normalized := strings.TrimSpace(multiWhitespace.ReplaceAllString(body, " "))
	h := sha256.New()
	h.Write([]byte(normalized))
	h.Write([]byte{0})
	h.Write([]byte(illustrationHash))
	return hex.EncodeToString(h.Sum(nil))
}
