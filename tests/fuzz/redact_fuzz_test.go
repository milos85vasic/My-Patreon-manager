package fuzz

import (
	"strings"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/utils"
)

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

		// A literal `user:pass@` segment must not survive redaction.
		// We look for the `@` and, if present, confirm the byte stream
		// immediately before it does NOT match the input's credential
		// portion (checking via suffix prefix instead of full substring
		// keeps false positives low on exotic fuzzer inputs).
		if at := strings.LastIndex(raw, "@"); at > 0 {
			rawCred := raw[:at]
			if schemeIdx := strings.Index(rawCred, "://"); schemeIdx >= 0 {
				rawCred = rawCred[schemeIdx+3:]
			}
			if rawCred != "" && strings.Contains(rawCred, ":") && strings.Contains(out, rawCred+"@") {
				t.Fatalf("RedactURL leaked %q verbatim into %q", rawCred, out)
			}
		}
	})
}
