package process_test

import (
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/services/process"
)

func TestFingerprint_Deterministic(t *testing.T) {
	a := process.Fingerprint("hello world", "abc")
	b := process.Fingerprint("hello world", "abc")
	if a != b {
		t.Fatalf("not deterministic: %s != %s", a, b)
	}
}

func TestFingerprint_WhitespaceInsensitive(t *testing.T) {
	a := process.Fingerprint("hello\n\n  world", "abc")
	b := process.Fingerprint("hello world", "abc")
	if a != b {
		t.Fatalf("whitespace sensitivity: %s != %s", a, b)
	}
}

func TestFingerprint_EmptyIllustration(t *testing.T) {
	a := process.Fingerprint("body", "")
	if len(a) != 64 {
		t.Fatalf("expected 64-char sha256 hex, got %d chars", len(a))
	}
}

func TestFingerprint_IllustrationMatters(t *testing.T) {
	a := process.Fingerprint("body", "illust1")
	b := process.Fingerprint("body", "illust2")
	if a == b {
		t.Fatalf("illustration hash should change fingerprint")
	}
}

func TestFingerprint_BodyMatters(t *testing.T) {
	a := process.Fingerprint("body-one", "x")
	b := process.Fingerprint("body-two", "x")
	if a == b {
		t.Fatalf("body change should change fingerprint")
	}
}

func TestFingerprint_LeadingTrailingWhitespaceNormalized(t *testing.T) {
	a := process.Fingerprint("  body  ", "x")
	b := process.Fingerprint("body", "x")
	if a != b {
		t.Fatalf("leading/trailing whitespace should be normalized: %s != %s", a, b)
	}
}

func TestFingerprint_CrossCollisionBodyIllust(t *testing.T) {
	// Body "ax" + illust "bc" must not collide with body "a" + illust "xbc".
	// This guards against naive concatenation attacks on the separator.
	a := process.Fingerprint("ax", "bc")
	b := process.Fingerprint("a", "xbc")
	if a == b {
		t.Fatalf("separator missing — bodies with concat-ambiguity collide")
	}
}
