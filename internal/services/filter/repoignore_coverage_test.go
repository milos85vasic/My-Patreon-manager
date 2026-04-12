package filter

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMatch_BasicPatterns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".repoignore")
	content := "github.com/owner/repo\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	r, err := ParseRepoignoreFile(path)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		url     string
		matched bool
	}{
		{"exact match", "https://github.com/owner/repo", true},
		{"with .git suffix", "https://github.com/owner/repo.git", true},
		{"no match", "https://github.com/other/repo2", false},
		{"empty url", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.Match(tt.url)
			if got != tt.matched {
				t.Errorf("Match(%q) = %v, want %v", tt.url, got, tt.matched)
			}
		})
	}
}

func TestMatch_NegationPattern(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".repoignore")
	content := "github.com/owner/*\n!github.com/owner/keepme\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	r, err := ParseRepoignoreFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if !r.Match("https://github.com/owner/somerepo") {
		t.Error("expected match for github.com/owner/somerepo")
	}
	if r.Match("https://github.com/owner/keepme") {
		t.Error("expected no match for negated github.com/owner/keepme")
	}
}

func TestMatch_WildcardPattern(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".repoignore")
	content := "github.com/owner/*-old\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	r, err := ParseRepoignoreFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if !r.Match("https://github.com/owner/myrepo-old") {
		t.Error("expected match for wildcard *-old")
	}
	if r.Match("https://github.com/owner/myrepo-new") {
		t.Error("expected no match for *-old on -new")
	}
}

func TestMatch_RecursivePattern(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".repoignore")
	content := "github.com/**/test\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	r, err := ParseRepoignoreFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if !r.Match("https://github.com/owner/test") {
		t.Error("expected match for recursive pattern")
	}
	if r.Match("https://github.com/owner/other") {
		t.Error("expected no match for non-matching recursive")
	}
}

func TestMatch_RecursivePatternPartNoMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".repoignore")
	content := "github.com/**/specific-name\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	r, err := ParseRepoignoreFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if r.Match("https://github.com/owner/different-name") {
		t.Error("expected no match when recursive part doesn't match")
	}
}

func TestMatch_DoubleStarAlone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".repoignore")
	content := "**\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	r, err := ParseRepoignoreFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if !r.Match("https://github.com/anything") {
		t.Error("** should match everything")
	}
}

func TestMatch_CharClassPattern(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".repoignore")
	content := "github.com/owner/repo[123]\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	r, err := ParseRepoignoreFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if !r.Match("https://github.com/owner/repo1") {
		t.Error("expected match for char class [123] with '1'")
	}
	if r.Match("https://github.com/owner/repo5") {
		t.Error("expected no match for char class [123] with '5'")
	}
}

func TestMatch_CharClassPrefix(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".repoignore")
	content := "github.com/owner/x[ab]y\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	r, err := ParseRepoignoreFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if !r.Match("https://github.com/owner/xay") {
		t.Error("expected match for x[ab]y with 'a'")
	}
	if r.Match("https://github.com/owner/xcy") {
		t.Error("expected no match for x[ab]y with 'c'")
	}
}

func TestMatch_CharClassPrefixNoMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".repoignore")
	content := "zzz/[ab]\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	r, err := ParseRepoignoreFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// prefix doesn't match
	if r.Match("https://github.com/owner/repo") {
		t.Error("expected no match when prefix doesn't match")
	}
}

func TestMatch_CharClassIdxOutOfRange(t *testing.T) {
	// Edge case: prefix is the entire URL
	r := &Repoignore{}
	// Direct call: url is exactly the prefix, so idx >= len(url)
	result := r.matchCharClass("abc", "abc[x]")
	if result {
		t.Error("expected false when idx >= len(url)")
	}
}

func TestMatch_CharClassSuffixLongerThanRemaining(t *testing.T) {
	r := &Repoignore{}
	// suffix longer than what's left: url="abcd", pattern="ab[x]efgh"
	result := r.matchCharClass("abcd", "ab[x]efgh")
	if result {
		t.Error("expected false when suffix longer than remaining url")
	}
}

func TestMatch_CharClassSuffixMismatch(t *testing.T) {
	r := &Repoignore{}
	// suffix doesn't match: url="abcd", pattern="ab[c]e" — suffix is "e", url[3:] is "d"
	result := r.matchCharClass("abcd", "ab[c]e")
	if result {
		t.Error("expected false when suffix doesn't match")
	}
}

func TestMatchWildcard_MoreThanTwoParts(t *testing.T) {
	r := &Repoignore{}
	// pattern with two *: "a*b*c" splits into 3 parts, not 2
	result := r.matchWildcard("axbxc", "a*b*c")
	if result {
		t.Error("matchWildcard should return false for pattern with > 2 parts")
	}
}

func TestReload_EmptyPath(t *testing.T) {
	r := &Repoignore{path: ""}
	err := r.Reload()
	if err != nil {
		t.Fatalf("Reload with empty path should return nil: %v", err)
	}
}

func TestReload_ValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".repoignore")
	if err := os.WriteFile(path, []byte("github.com/owner/repo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	r, err := ParseRepoignoreFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Match("https://github.com/owner/repo") {
		t.Error("expected match before reload")
	}

	// Update file
	if err := os.WriteFile(path, []byte("github.com/owner/other\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Reload(); err != nil {
		t.Fatal(err)
	}
	if r.Match("https://github.com/owner/repo") {
		t.Error("expected no match after reload")
	}
	if !r.Match("https://github.com/owner/other") {
		t.Error("expected match for new pattern after reload")
	}
}

func TestValidatePatterns(t *testing.T) {
	patterns := []Pattern{
		{Raw: "github.com/owner/repo[123"},
		{Raw: "github.com/owner/repo]"},
		{Raw: "github.com/owner/ok "},
		{Raw: "github.com/owner/valid"},
	}
	issues := ValidatePatterns(patterns)
	if len(issues) != 3 {
		t.Fatalf("expected 3 issues, got %d: %v", len(issues), issues)
	}
}

func TestNormalizeForMatch(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://github.com/Owner/Repo.git", "github.com/owner/repo"},
		{"http://github.com/Owner/Repo", "github.com/owner/repo"},
		{"git@github.com:Owner/Repo.git", "github.com/owner/repo"},
		{"  https://github.com/owner/repo  ", "github.com/owner/repo"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeForMatch(tt.input)
			if got != tt.expected {
				t.Errorf("normalizeForMatch(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseRepoignoreFile_NonExistent(t *testing.T) {
	r, err := ParseRepoignoreFile("/nonexistent/.repoignore")
	if err != nil {
		t.Fatal(err)
	}
	if r == nil {
		t.Fatal("expected non-nil Repoignore for missing file")
	}
}

func TestParseRepoignoreFile_CommentsAndBlanks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".repoignore")
	content := "# comment\n\ngithub.com/owner/repo\n# another comment\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	r, err := ParseRepoignoreFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Match("https://github.com/owner/repo") {
		t.Error("expected match")
	}
}

func TestParseRepoignoreFile_RecursiveSuffix(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".repoignore")
	content := "github.com/owner/**\ngithub.com/other/**/\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	r, err := ParseRepoignoreFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Match("https://github.com/owner/anything") {
		t.Error("expected match for /** suffix")
	}
}

func TestWatchSIGHUP_ReloadError(t *testing.T) {
	// Create a file, then make it unreadable so Reload returns an error.
	dir := t.TempDir()
	path := filepath.Join(dir, ".repoignore")
	if err := os.WriteFile(path, []byte("github.com/owner/repo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	r, err := ParseRepoignoreFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// Make file unreadable so Reload will fail
	if err := os.Chmod(path, 0000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(path, 0644)

	// We need to control the signal channel directly to ensure the reload
	// error path is exercised before we stop the watcher.
	reloadDone := make(chan struct{})
	origNotify := SignalNotify
	defer func() { SignalNotify = origNotify }()

	var fakeCh chan<- os.Signal
	SignalNotify = func(c chan<- os.Signal, sig ...os.Signal) {
		fakeCh = c
	}

	stop := make(chan struct{})
	done := r.WatchSIGHUP(stop)

	// Send signal and wait for it to be processed
	go func() {
		fakeCh <- os.Interrupt // any signal, just to trigger the case
		close(reloadDone)
	}()
	<-reloadDone

	// Give the goroutine time to process
	time.Sleep(10 * time.Millisecond)

	close(stop)
	<-done
}

func TestReload_Error(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".repoignore")
	if err := os.WriteFile(path, []byte("github.com/owner/repo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	r, err := ParseRepoignoreFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Make file unreadable
	if err := os.Chmod(path, 0000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(path, 0644)

	err = r.Reload()
	if err == nil {
		t.Error("expected error on reload when file is unreadable")
	}
}

func TestMatchPattern_SubstringContains(t *testing.T) {
	r := &Repoignore{}
	// pattern without wildcards, brackets, or ** — simple substring contains
	p := Pattern{Pattern: "owner/repo"}
	if !r.matchPattern("github.com/owner/repo", p) {
		t.Error("expected substring match")
	}
	if r.matchPattern("github.com/other/thing", p) {
		t.Error("expected no match for non-matching substring")
	}
}

func TestMatchCharClass_BracketsInvalid(t *testing.T) {
	r := &Repoignore{}
	// end before start — should return false
	result := r.matchCharClass("test", "]x[")
	if result {
		t.Error("expected false for invalid bracket order")
	}
}

func TestMatchRecursive_EmptyParts(t *testing.T) {
	r := &Repoignore{}
	// Pattern "**/**" after replacements becomes "*" then split produces empty parts
	// which should be skipped via the continue branch
	result := r.matchRecursive("github.com/owner/repo", "**/**")
	if !result {
		t.Error("expected match for pattern with empty parts after splitting")
	}
}

func TestParseRepoignoreFile_WithValidationIssues(t *testing.T) {
	// File that triggers validation issues (e.g. trailing whitespace on patterns)
	dir := t.TempDir()
	path := filepath.Join(dir, ".repoignore")
	content := "github.com/owner/repo \n" // trailing space triggers issue
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	r, err := ParseRepoignoreFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Pattern should still be loaded (after trimming)
	if !r.Match("https://github.com/owner/repo") {
		t.Error("expected match after trimming")
	}
}

func TestParseRepoignoreFile_InvalidBracketLogsWarning(t *testing.T) {
	// File with unclosed bracket — triggers filterValidPatterns to return issues,
	// which ParseRepoignoreFile logs via slog.Warn.
	dir := t.TempDir()
	path := filepath.Join(dir, ".repoignore")
	content := "github.com/owner/repo[abc\ngithub.com/valid/repo\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	r, err := ParseRepoignoreFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// The invalid pattern is dropped; valid one remains
	if !r.Match("https://github.com/valid/repo") {
		t.Error("expected match for valid pattern")
	}
}
