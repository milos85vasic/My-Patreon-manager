package fuzz

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/services/filter"
)

func FuzzRepoignoreMatch(f *testing.F) {
	f.Add("*.go")
	f.Add("!vendor/**")
	f.Add("")
	f.Add("**/*_test.go")
	f.Add("github.com/owner/repo")
	f.Add("[abc]")
	f.Add("prefix*suffix")
	f.Add("**/deep/path/**")
	f.Add("!negated")
	f.Add("a[b")    // unclosed bracket
	f.Add("a]b")    // unmatched bracket
	f.Add("a\x00b") // null byte

	f.Fuzz(func(t *testing.T, pattern string) {
		// Write a repoignore file with the fuzzed pattern
		dir := t.TempDir()
		path := filepath.Join(dir, ".repoignore")
		if err := os.WriteFile(path, []byte(pattern+"\n"), 0644); err != nil {
			return
		}
		r, err := filter.ParseRepoignoreFile(path)
		if err != nil {
			return // parse errors are acceptable
		}
		// Exercise Match — should never panic
		_ = r.Match("any/path/file.go")
		_ = r.Match("https://github.com/owner/repo.git")
		_ = r.Match("")
	})
}
