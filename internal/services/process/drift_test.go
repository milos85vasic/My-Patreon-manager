package process_test

import (
	"context"
	"errors"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/services/process"
)

func TestDriftFingerprint_Deterministic(t *testing.T) {
	if process.DriftFingerprint("hello") != process.DriftFingerprint("hello") {
		t.Fatal("not deterministic")
	}
}

func TestDriftFingerprint_Length(t *testing.T) {
	if len(process.DriftFingerprint("x")) != 64 {
		t.Fatal("want 64-char hex")
	}
}

func TestDriftFingerprint_WhitespaceNormalized(t *testing.T) {
	a := process.DriftFingerprint("<p>hello  world</p>\n\n")
	b := process.DriftFingerprint("<p>hello world</p>")
	if a != b {
		t.Fatalf("whitespace not normalized: %s != %s", a, b)
	}
}

func TestDriftFingerprint_HTMLTagBoundaryNormalized(t *testing.T) {
	a := process.DriftFingerprint("<p>x</p>\n<p>y</p>")
	b := process.DriftFingerprint("<p>x</p><p>y</p>")
	if a != b {
		t.Fatalf("tag-boundary not normalized: %s != %s", a, b)
	}
}

func TestDriftFingerprint_ContentChangesShowAsDrift(t *testing.T) {
	a := process.DriftFingerprint("<p>hello</p>")
	b := process.DriftFingerprint("<p>goodbye</p>")
	if a == b {
		t.Fatal("real content change not detected")
	}
}

func TestDriftFingerprint_LeadingTrailingTrimmed(t *testing.T) {
	a := process.DriftFingerprint("   <p>x</p>   ")
	b := process.DriftFingerprint("<p>x</p>")
	if a != b {
		t.Fatalf("leading/trailing not trimmed: %s != %s", a, b)
	}
}

func TestDriftChecker_NoDrift(t *testing.T) {
	check := process.DriftChecker(func(ctx context.Context, postID string) (string, error) {
		return "<p>hello  world</p>", nil
	})
	expected := process.DriftFingerprint("<p>hello world</p>")
	drift, err := check(context.Background(), "p1", expected)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if drift {
		t.Fatal("want no drift")
	}
}

func TestDriftChecker_Drift(t *testing.T) {
	check := process.DriftChecker(func(ctx context.Context, postID string) (string, error) {
		return "<p>edited externally</p>", nil
	})
	expected := process.DriftFingerprint("<p>original</p>")
	drift, err := check(context.Background(), "p1", expected)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !drift {
		t.Fatal("want drift")
	}
}

func TestDriftChecker_FetchError(t *testing.T) {
	check := process.DriftChecker(func(ctx context.Context, postID string) (string, error) {
		return "", errors.New("boom")
	})
	_, err := check(context.Background(), "p1", "fp")
	if err == nil {
		t.Fatal("expected fetch error to propagate")
	}
}
