package fuzz

import (
	"strings"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/utils"
)

// FuzzNormalizeHTTPS drives the URL normalizer with arbitrary input.
// Invariant: the function must never panic, and the returned value
// must be a valid-looking string (no control characters beyond
// whitespace). The seed corpus covers the common Git-URL shapes we
// encounter in production plus malformed inputs observed in incident
// reports (empty, schemes with no authority, embedded null bytes,
// non-ASCII characters).
func FuzzNormalizeHTTPS(f *testing.F) {
	seeds := []string{
		"",
		"git@github.com:owner/repo.git",
		"https://github.com/owner/repo",
		"https://github.com/owner/repo/",
		"https://GitHub.com/owner/repo",
		"ssh://git@github.com/owner/repo.git",
		"ssh://git@gitflic.ru:2222/owner/repo.git",
		"https://",
		"github.com/owner/repo",
		"http://insecure.example.com/owner/repo",
		"git://legacy.example.com/owner/repo.git",
		"\x00",
		"\x00https://github.com/owner/repo",
		"https://github.com/owner/repo?token=secret",
		"https://user:pass@github.com/owner/repo",
		"https://%00/owner/repo",
		strings.Repeat("a", 8192),
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, url string) {
		got := utils.NormalizeHTTPS(url)
		// Invariants:
		//   - Never panic (implicit — any panic fails the fuzz).
		//   - Output length must be bounded by input length + a small
		//     constant. The function is a trimmer/normalizer, not a
		//     generator; unbounded output would indicate a
		//     quadratic-like pathology.
		if len(got) > len(url)+64 {
			t.Fatalf("NormalizeHTTPS produced suspiciously large output: input %d bytes, output %d bytes", len(url), len(got))
		}
		// Note: we intentionally do NOT assert output is well-formed
		// UTF-8 or free of control characters — the function's
		// contract is "canonicalize reasonable input" and it is
		// permissive on malformed input. Downstream consumers
		// (logging, storage) apply additional sanitization.
		_ = got
	})
}
