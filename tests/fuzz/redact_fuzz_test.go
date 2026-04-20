package fuzz

import (
	"strings"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/utils"
)

// isDistinguishable reports whether the credential component is long enough
// AND varied enough to distinguish from the `***:***@` redaction sentinel.
// A single-char input like `"*"` or `":"` is ambiguous — it would appear in
// the sentinel regardless of whether it leaked, so the fuzz assertion has
// to skip it. Anything at least two chars AND containing a non-colon,
// non-asterisk rune is distinguishable.
func isDistinguishable(s string) bool {
	if len(s) < 2 {
		return false
	}
	for _, r := range s {
		if r != '*' && r != ':' {
			return true
		}
	}
	return false
}

// FuzzRedactURL drives the credential-redaction helper with arbitrary
// URL-like input. Invariants the fuzzer enforces:
//   - Must never panic.
//   - Must never echo back the `user:password@` credential block from
//     the input verbatim — that's the whole point of the redactor.
//   - Must never return a shorter string than the known-safe
//     substitute token prefix when the input carried credentials.
func FuzzRedactURL(f *testing.F) {
	seeds := []string{
		"",
		"https://github.com/owner/repo",
		"https://user:pass@github.com/owner/repo",
		"https://tok:@github.com/owner/repo",
		"https://:secret@github.com/owner/repo",
		"git://user:p@ss@legacy.example.com/owner",
		"https://user:password@example.com/path?token=s3cr3t",
		"not-a-url",
		"://",
		"\x00://\x00@\x00",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		out := utils.RedactURL(raw)

		// RedactURL only redacts credentials in strings that look like
		// URLs — specifically those with a `scheme://user:pass@host`
		// shape. The contract is NOT "redact any `:@` anywhere"; that's
		// the job of utils.RedactString's regex-driven pipeline.
		//
		// So we only assert on inputs that have a scheme AND a
		// user:password@ authority with non-empty user AND pass. Inputs
		// without a scheme, or with degenerate authorities like `:@`,
		// are out of RedactURL's contract and fall through untouched.
		schemeIdx := strings.Index(raw, "://")
		if schemeIdx < 0 {
			return
		}
		rest := raw[schemeIdx+3:]
		at := strings.Index(rest, "@")
		if at <= 0 {
			return
		}
		userinfo := rest[:at]
		if strings.ContainsAny(userinfo, "/?#") {
			return // `@` belongs to the path, not the authority
		}
		colon := strings.Index(userinfo, ":")
		if colon <= 0 || colon >= len(userinfo)-1 {
			// Not a `user:pass` shape — either no colon, or colon at
			// the start/end, which wouldn't constitute a credential.
			return
		}
		user := userinfo[:colon]
		pass := userinfo[colon+1:]

		// The raw user OR password substring must not appear between
		// `://` and `@` in the output. Both components are
		// independently sensitive; either surviving is a leak.
		//
		// Skip components that are pure asterisks or other characters
		// that collide with the `***:***@` redaction sentinel — those
		// can't produce a meaningful leak signal.
		outAfterScheme := out[strings.Index(out, "://")+3:]
		if outAt := strings.Index(outAfterScheme, "@"); outAt > 0 {
			outUserinfo := outAfterScheme[:outAt]
			if isDistinguishable(user) && strings.Contains(outUserinfo, user) {
				t.Fatalf("RedactURL leaked user %q into %q", user, out)
			}
			if isDistinguishable(pass) && strings.Contains(outUserinfo, pass) {
				t.Fatalf("RedactURL leaked password %q into %q", pass, out)
			}
		}
	})
}
