package utils

import (
	"regexp"
	"strings"
)

var sensitivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(token|secret|key|password|auth)[\s]*[=:]\s*["']?([^"'\s,]+)["']?`),
	regexp.MustCompile(`(?i)bearer\s+([a-zA-Z0-9_\-\.]+)`),
	regexp.MustCompile(`gh[pors]_[a-zA-Z0-9]{36,}`),
	regexp.MustCompile(`glpat-[a-zA-Z0-9_\-]{20}`),
}

var defaultSensitivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(token|secret|key|password|auth)[\s]*[=:]\s*["']?([^"'\s,]+)["']?`),
	regexp.MustCompile(`(?i)bearer\s+([a-zA-Z0-9_\-\.]+)`),
	regexp.MustCompile(`gh[pors]_[a-zA-Z0-9]{36,}`),
	regexp.MustCompile(`glpat-[a-zA-Z0-9_\-]{20}`),
}

func RedactString(s string) string {
	return RedactStringWithPatterns(s, defaultSensitivePatterns)
}

func RedactStringWithPatterns(s string, patterns []*regexp.Regexp) string {
	result := s
	for _, pattern := range patterns {
		result = redactWithPattern(result, pattern)
	}
	return result
}

// redactWithPattern replaces sensitive parts matched by pattern with asterisks.
// If the pattern contains capturing groups, the last capturing group is considered
// the sensitive value and only that group is redacted. Otherwise the entire match
// is redacted.
func redactWithPattern(s string, pattern *regexp.Regexp) string {
	matches := pattern.FindAllStringSubmatchIndex(s, -1)
	if len(matches) == 0 {
		return s
	}
	// Build new string by copying parts, replacing matched groups.
	var buf strings.Builder
	last := 0
	for _, match := range matches {
		// match is a slice of pairs [start, end] for whole match and each subgroup.
		// subgroup indices start at index 2.
		groupCount := len(match) / 2
		if groupCount <= 1 {
			// No capturing groups or only whole match: redact whole match.
			start, end := match[0], match[1]
			buf.WriteString(s[last:start])
			buf.WriteString(strings.Repeat("*", end-start))
			last = end
		} else {
			// At least one capturing group. Redact the last capturing group.
			// The last group indices are at match[len(match)-2], match[len(match)-1]
			start, end := match[len(match)-2], match[len(match)-1]
			// Write prefix before whole match
			buf.WriteString(s[last:match[0]])
			// Write part of match before the redacted group
			buf.WriteString(s[match[0]:start])
			// Write redaction
			buf.WriteString(strings.Repeat("*", end-start))
			// Write part after redacted group (if any)
			// Actually after redacted group, the match continues until match[1].
			// But there may be text after the group within the match (e.g., closing quote).
			// We'll write from end to match[1].
			buf.WriteString(s[end:match[1]])
			last = match[1]
		}
	}
	buf.WriteString(s[last:])
	return buf.String()
}

// RedactURL replaces the two parts of a URL that commonly carry
// secrets: the `user:password@` userinfo block in the authority and
// any query string. The scheme, host, path, and fragment are left
// intact so logs stay useful for debugging.
//
// Inputs without a scheme (e.g. `github.com/owner/repo`) are returned
// unchanged; inputs without userinfo or a query string fall through
// the no-op path. The function is intentionally tolerant of malformed
// input — it never returns an error — because it sits on the logging
// path where a panic would lose the caller's context.
//
// This function was surfaced by the FuzzRedactURL target as a
// historical security gap: the previous implementation only stripped
// the query string, so a URL like
// `https://user:password@github.com/owner/repo` was logged verbatim.
func RedactURL(url string) string {
	// 1. Redact the userinfo (`user:password@` / `:password@` /
	// `token@`) between the scheme separator and the host. `scheme://`
	// is required — we leave schemeless strings alone because they
	// are ambiguous between URLs and log messages.
	if schemeEnd := strings.Index(url, "://"); schemeEnd >= 0 {
		rest := url[schemeEnd+3:]
		if at := strings.Index(rest, "@"); at >= 0 {
			// Only treat rest[:at] as userinfo if it does not contain
			// a `/` — otherwise the `@` belongs to the path.
			userinfo := rest[:at]
			if !strings.ContainsAny(userinfo, "/?#") {
				url = url[:schemeEnd+3] + "***:***@" + rest[at+1:]
			}
		}
	}

	// 2. Redact the query string.
	if strings.Contains(url, "?") {
		parts := strings.SplitN(url, "?", 2)
		return parts[0] + "?***"
	}
	return url
}
