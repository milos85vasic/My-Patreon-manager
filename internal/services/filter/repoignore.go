package filter

import (
	"bufio"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"unicode"
)

type Pattern struct {
	Pattern     string
	IsNegation  bool
	IsRecursive bool
	Raw         string
}

type Repoignore struct {
	patterns []Pattern
	mu       sync.RWMutex
	path     string
}

func ParseRepoignoreFile(path string) (*Repoignore, error) {
	r := &Repoignore{path: path}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return r, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	var rawPatterns []Pattern
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), " \t\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		pattern := Pattern{Raw: line}
		if strings.HasPrefix(line, "!") {
			pattern.IsNegation = true
			pattern.Pattern = line[1:]
		} else {
			pattern.Pattern = line
		}
		if strings.HasSuffix(pattern.Pattern, "/**") || strings.HasSuffix(pattern.Pattern, "/**/") {
			pattern.IsRecursive = true
		}
		rawPatterns = append(rawPatterns, pattern)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	// Filter invalid patterns and log warnings
	filtered, issues := filterValidPatterns(rawPatterns)
	for _, issue := range issues {
		slog.Warn("repoignore validation", "issue", issue)
	}
	r.patterns = filtered
	return r, nil
}

func (r *Repoignore) Match(repoURL string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	url := normalizeForMatch(repoURL)
	matched := false

	for _, p := range r.patterns {
		if r.matchPattern(url, p) {
			if p.IsNegation {
				matched = false
			} else {
				matched = true
			}
		}
	}

	return matched
}

func (r *Repoignore) matchPattern(url string, p Pattern) bool {
	pattern := p.Pattern
	pattern = strings.TrimSuffix(pattern, "/**")
	pattern = strings.TrimSuffix(pattern, "/")
	if pattern == "**" {
		return true
	}
	if strings.Contains(pattern, "**") {
		return r.matchRecursive(url, pattern)
	}
	if strings.Contains(pattern, "*") {
		return r.matchWildcard(url, pattern)
	}
	if strings.Contains(pattern, "[") && strings.Contains(pattern, "]") {
		return r.matchCharClass(url, pattern)
	}
	return strings.Contains(url, pattern)
}

func (r *Repoignore) matchWildcard(url, pattern string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 2 {
		return strings.HasPrefix(url, parts[0]) && strings.HasSuffix(url, parts[1])
	}
	return false
}

func (r *Repoignore) matchRecursive(url, pattern string) bool {
	pattern = strings.ReplaceAll(pattern, "**/", "*")
	pattern = strings.ReplaceAll(pattern, "/**", "")
	parts := strings.Split(pattern, "*")
	for _, part := range parts {
		if part == "" {
			continue
		}
		if !strings.Contains(url, part) {
			return false
		}
	}
	return true
}

func (r *Repoignore) matchCharClass(url, pattern string) bool {
	start := strings.Index(pattern, "[")
	end := strings.Index(pattern, "]")
	if start == -1 || end == -1 || end <= start {
		return false
	}
	prefix := pattern[:start]
	suffix := pattern[end+1:]
	class := pattern[start+1 : end]
	if !strings.HasPrefix(url, prefix) {
		return false
	}
	if suffix != "" && !strings.HasSuffix(url, suffix) {
		return false
	}
	idx := len(prefix)
	if idx >= len(url) {
		return false
	}
	for _, c := range class {
		if unicode.ToLower(c) == unicode.ToLower(rune(url[idx])) {
			return true
		}
	}
	return false
}

func (r *Repoignore) Reload() error {
	if r.path == "" {
		return nil
	}
	newR, err := ParseRepoignoreFile(r.path)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.patterns = newR.patterns
	r.mu.Unlock()
	return nil
}

func (r *Repoignore) WatchSIGHUP() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGHUP)
	go func() {
		for range ch {
			if err := r.Reload(); err != nil {
				slog.Error("repoignore reload failed", "error", err)
			}
		}
	}()
}

func ValidatePatterns(patterns []Pattern) []string {
	var issues []string
	for _, p := range patterns {
		raw := p.Raw
		if strings.Contains(raw, "[") && !strings.Contains(raw, "]") {
			issues = append(issues, "unclosed bracket: "+raw)
		}
		if strings.Contains(raw, "]") && !strings.Contains(raw, "[") {
			issues = append(issues, "unmatched closing bracket: "+raw)
		}
		trimmed := strings.TrimRight(raw, " \t")
		if trimmed != raw {
			issues = append(issues, "trailing whitespace: "+raw)
		}
	}
	return issues
}

// filterValidPatterns filters out invalid patterns and returns the filtered slice and any validation issues.
// Invalid patterns are those with unclosed brackets or unmatched closing brackets.
// Patterns with trailing whitespace are kept after trimming.
func filterValidPatterns(patterns []Pattern) ([]Pattern, []string) {
	var valid []Pattern
	var issues []string
	for _, p := range patterns {
		raw := p.Raw
		// Check for bracket errors
		if strings.Contains(raw, "[") && !strings.Contains(raw, "]") {
			issues = append(issues, "unclosed bracket: "+raw)
			continue
		}
		if strings.Contains(raw, "]") && !strings.Contains(raw, "[") {
			issues = append(issues, "unmatched closing bracket: "+raw)
			continue
		}
		// Trim trailing whitespace from raw line and adjust pattern fields
		trimmed := strings.TrimRight(raw, " \t")
		if trimmed != raw {
			issues = append(issues, "trailing whitespace: "+raw)
			// Update the pattern with trimmed raw and pattern fields
			p.Raw = trimmed
			if p.IsNegation {
				p.Pattern = trimmed[1:]
			} else {
				p.Pattern = trimmed
			}
			// Re-evaluate recursive suffix after trimming
			p.IsRecursive = false
			if strings.HasSuffix(p.Pattern, "/**") || strings.HasSuffix(p.Pattern, "/**/") {
				p.IsRecursive = true
			}
		}
		valid = append(valid, p)
	}
	return valid, issues
}

func normalizeForMatch(url string) string {
	url = strings.TrimSpace(url)
	url = strings.TrimSuffix(url, ".git")
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimPrefix(url, "git@")
	url = strings.ReplaceAll(url, ":", "/")
	url = strings.ReplaceAll(url, "//", "/")
	return strings.ToLower(url)
}
