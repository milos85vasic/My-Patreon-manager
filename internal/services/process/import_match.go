package process

import (
	"regexp"
	"strings"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

// matchRepoLayered resolves the target repo for a PatreonPost using a
// four-layer heuristic. Layers are tried in order and the first match
// wins. Precedence (strong → weak):
//
//  1. Explicit tag:   `repo:<id>` substring in post.Content where <id>
//                     matches a repo.ID case-insensitively.
//  2. Embedded URL:   a repo's URL or HTTPSURL appears in post.Content
//                     (case-insensitive, trailing-slash-insensitive).
//  3. Slug in title:  `owner/name` or `name` appears in post.Title as a
//                     whole word (regex \b boundaries).
//  4. Substring:      case-insensitive substring of repo.Name in
//                     post.Title — the legacy v1 heuristic, kept as a
//                     fuzzy fallback.
//
// Returns nil when no layer matches; callers route nil-matches to the
// unmatched_patreon_posts workflow.
func matchRepoLayered(post PatreonPost, repos []*models.Repository) *models.Repository {
	if r := matchByTag(post, repos); r != nil {
		return r
	}
	if r := matchByURL(post, repos); r != nil {
		return r
	}
	if r := matchBySlug(post, repos); r != nil {
		return r
	}
	if r := matchBySubstring(post, repos); r != nil {
		return r
	}
	return nil
}

// matchByTag scans post.Content for a `repo:<id>` substring where <id>
// equals one of the candidate repo IDs (case-insensitive). Empty repo
// IDs are skipped so a malformed row can't match every post.
func matchByTag(post PatreonPost, repos []*models.Repository) *models.Repository {
	if post.Content == "" {
		return nil
	}
	lower := strings.ToLower(post.Content)
	for _, r := range repos {
		if r == nil || r.ID == "" {
			continue
		}
		needle := "repo:" + strings.ToLower(r.ID)
		if strings.Contains(lower, needle) {
			return r
		}
	}
	return nil
}

// matchByURL scans post.Content for any candidate repo's URL or
// HTTPSURL. Comparison is done on normalized forms (lowercase,
// trailing slash trimmed) against the lowercased content; this lets
// a stored URL without a trailing slash match a content URL with one
// (and vice versa).
//
// Candidates that don't look like URLs — no `://` (HTTPS/HTTP) and no
// `@` (git SSH) — are skipped. Without that filter a placeholder like
// "u" or "h" in the repos table would substring-match nearly any post
// body and swallow traffic the later layers should handle.
func matchByURL(post PatreonPost, repos []*models.Repository) *models.Repository {
	if post.Content == "" {
		return nil
	}
	lowerContent := strings.ToLower(post.Content)
	for _, r := range repos {
		if r == nil {
			continue
		}
		for _, candidate := range []string{r.URL, r.HTTPSURL} {
			if !looksLikeURL(candidate) {
				continue
			}
			norm := normalizeURL(candidate)
			// looksLikeURL already guaranteed len >= 8 and a `://` or
			// `@`, so norm cannot be empty here — no extra guard needed.
			if strings.Contains(lowerContent, norm) {
				return r
			}
		}
	}
	return nil
}

// looksLikeURL is a cheap guard that rejects obvious non-URLs (empty,
// very short, or missing both a scheme separator and an @). Anything
// that passes this check is safe to substring-match against post
// bodies.
func looksLikeURL(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) < 8 {
		return false
	}
	return strings.Contains(s, "://") || strings.Contains(s, "@")
}

// matchBySlug looks for `owner/name` or `name` as a whole word in
// post.Title (case-insensitive). Whole-word means bounded by
// non-word characters — so a repo named `hello-world` won't match a
// title like "releasehelloworldv1" here (that case falls through to
// the substring layer).
func matchBySlug(post PatreonPost, repos []*models.Repository) *models.Repository {
	if post.Title == "" {
		return nil
	}
	for _, r := range repos {
		if r == nil || r.Name == "" {
			continue
		}
		if r.Owner != "" {
			full := r.Owner + "/" + r.Name
			if containsWholeWord(post.Title, full) {
				return r
			}
		}
		if containsWholeWord(post.Title, r.Name) {
			return r
		}
	}
	return nil
}

// matchBySubstring preserves the v1 heuristic: case-insensitive
// substring of repo.Name in post.Title. Acts as the fuzzy fallback
// after the stricter layers have failed.
func matchBySubstring(post PatreonPost, repos []*models.Repository) *models.Repository {
	if post.Title == "" {
		return nil
	}
	lowerTitle := strings.ToLower(post.Title)
	for _, r := range repos {
		if r == nil || r.Name == "" {
			continue
		}
		if strings.Contains(lowerTitle, strings.ToLower(r.Name)) {
			return r
		}
	}
	return nil
}

// normalizeURL lowercases the input and trims a single trailing slash.
// A full net/url parse isn't needed — our matching is substring-based
// and these two normalizations cover the common mismatches (casing
// in the host, stray trailing slash).
func normalizeURL(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimRight(s, "/")
	return s
}

// containsWholeWord reports whether needle occurs in haystack as a
// whole word (bounded by \b), case-insensitively. Regex
// metacharacters in needle are escaped so repo names containing `.`,
// `+`, `/`, etc. are handled safely. The resulting pattern is always
// a valid regexp, so MustCompile is safe — any panic here would
// indicate a bug in QuoteMeta itself.
func containsWholeWord(haystack, needle string) bool {
	if needle == "" {
		return false
	}
	pattern := `(?i)\b` + regexp.QuoteMeta(needle) + `\b`
	return regexp.MustCompile(pattern).MatchString(haystack)
}
