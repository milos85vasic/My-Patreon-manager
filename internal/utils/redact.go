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
		result = pattern.ReplaceAllStringFunc(result, func(match string) string {
			return strings.Repeat("*", len(match))
		})
	}
	return result
}

func RedactURL(url string) string {
	if strings.Contains(url, "?") {
		parts := strings.SplitN(url, "?", 2)
		return parts[0] + "?***"
	}
	return url
}
