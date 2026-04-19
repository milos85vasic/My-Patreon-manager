package process

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

// htmlTagWS matches whitespace between HTML tags. Collapsing this prevents
// benign Patreon re-rendering (which often introduces extra newlines
// between block-level tags) from registering as drift.
var htmlTagWS = regexp.MustCompile(`>\s+<`)

func normalizeForDrift(s string) string {
	s = htmlTagWS.ReplaceAllString(s, "><")
	s = multiWhitespace.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// DriftFingerprint returns a stable sha256 hex of the normalized content.
// Distinct from Fingerprint (which is for per-revision dedup of our own
// generated content + illustration hash). DriftFingerprint is symmetric:
// apply it to both our stored body and the body we fetch from Patreon,
// and compare hexes.
func DriftFingerprint(content string) string {
	h := sha256.Sum256([]byte(normalizeForDrift(content)))
	return hex.EncodeToString(h[:])
}

// FetchPostContent fetches a Patreon post's current body by post ID.
type FetchPostContent func(ctx context.Context, patreonPostID string) (string, error)

// DriftChecker returns a function that reports whether the Patreon-side
// content at postID has drifted away from the expectedFP we stored at
// publish time. A true return means drift was detected.
func DriftChecker(fetch FetchPostContent) func(ctx context.Context, postID, expectedFP string) (bool, error) {
	return func(ctx context.Context, postID, expectedFP string) (bool, error) {
		body, err := fetch(ctx, postID)
		if err != nil {
			return false, err
		}
		return DriftFingerprint(body) != expectedFP, nil
	}
}
