package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// profileFile writes a cover profile body to a temp file and returns the path.
func profileFile(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "prof.out")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	return p
}

func TestParseProfile_ValidAtomic(t *testing.T) {
	body := `mode: atomic
github.com/x/pkg/file.go:1.0,3.0 2 5
github.com/x/pkg/file.go:4.0,6.0 1 0
`
	mode, recs, err := parseProfile(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parseProfile: %v", err)
	}
	if mode != "atomic" {
		t.Fatalf("mode want atomic got %q", mode)
	}
	if len(recs) != 2 {
		t.Fatalf("want 2 records, got %d", len(recs))
	}
	for k, v := range recs {
		if k.file != "github.com/x/pkg/file.go" {
			t.Fatalf("unexpected file key: %q", k.file)
		}
		if v.numStmt == 0 {
			t.Fatalf("numStmt should be populated for %+v", k)
		}
	}
}

func TestParseProfile_DuplicateKeyTakesMax(t *testing.T) {
	body := `mode: atomic
a.go:1.0,2.0 1 3
a.go:1.0,2.0 1 9
a.go:1.0,2.0 1 1
`
	_, recs, err := parseProfile(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parseProfile: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("want 1 record; got %d", len(recs))
	}
	for _, v := range recs {
		if v.count != 9 {
			t.Fatalf("dup-key MAX want 9 got %d", v.count)
		}
	}
}

func TestParseProfile_MalformedLineErrors(t *testing.T) {
	cases := []string{
		"mode: atomic\nbad-no-colon-on-block 1 0\n",
		"mode: atomic\na.go:1.0,2.0 notanumber 0\n",
		"mode: atomic\na.go:1.0,2.0 1 notanumber\n",
		"mode: atomic\na.go:1.0,2.0 1\n", // missing count
	}
	for _, body := range cases {
		_, _, err := parseProfile(strings.NewReader(body))
		if err == nil {
			t.Fatalf("expected error for body:\n%s", body)
		}
	}
}

func TestParseProfile_MultipleModeHeadersConflict(t *testing.T) {
	body := "mode: atomic\nmode: set\n"
	_, _, err := parseProfile(strings.NewReader(body))
	if err == nil {
		t.Fatal("expected conflict error on mixed modes")
	}
}

func TestParseProfile_EmptyInputReturnsNoMode(t *testing.T) {
	mode, recs, err := parseProfile(strings.NewReader(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mode != "" {
		t.Fatalf("empty input must have no mode; got %q", mode)
	}
	if len(recs) != 0 {
		t.Fatalf("no records expected; got %d", len(recs))
	}
}

func TestMergeProfiles_MaxAcrossFiles(t *testing.T) {
	a := profileFile(t, `mode: atomic
a.go:1.0,2.0 1 2
a.go:3.0,4.0 1 0
`)
	b := profileFile(t, `mode: atomic
a.go:1.0,2.0 1 5
a.go:3.0,4.0 1 1
`)

	var buf bytes.Buffer
	if err := run([]string{a, b}, &buf); err != nil {
		t.Fatalf("run: %v", err)
	}
	out := buf.String()
	// Statement 1.0,2.0 should end up with count 5 (max of 2 and 5).
	if !strings.Contains(out, "a.go:1.0,2.0 1 5") {
		t.Fatalf("merge MAX failed for 1.0,2.0; output:\n%s", out)
	}
	if !strings.Contains(out, "a.go:3.0,4.0 1 1") {
		t.Fatalf("merge MAX failed for 3.0,4.0; output:\n%s", out)
	}
	// Exactly one mode header.
	if strings.Count(out, "mode:") != 1 {
		t.Fatalf("expected exactly one mode header; output:\n%s", out)
	}
}

func TestMergeProfiles_ConflictingModesError(t *testing.T) {
	a := profileFile(t, "mode: atomic\na.go:1.0,2.0 1 1\n")
	b := profileFile(t, "mode: set\na.go:1.0,2.0 1 1\n")
	err := run([]string{a, b}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected conflict error for mixed modes across files")
	}
}

func TestMergeProfiles_SkipsEmptyFile(t *testing.T) {
	a := profileFile(t, "")
	b := profileFile(t, "mode: atomic\na.go:1.0,2.0 1 3\n")
	var buf bytes.Buffer
	if err := run([]string{a, b}, &buf); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(buf.String(), "a.go:1.0,2.0 1 3") {
		t.Fatalf("output missing merged line:\n%s", buf.String())
	}
}

func TestRun_NoArgsError(t *testing.T) {
	if err := run(nil, &bytes.Buffer{}); err == nil {
		t.Fatal("want usage error on empty args")
	}
}

func TestRun_OpenMissingFileErrors(t *testing.T) {
	err := run([]string{"/nonexistent/path/that/cannot-exist-1234.out"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error opening missing file")
	}
}

func TestWriteMerged_EmptyMapEmitsValidHeader(t *testing.T) {
	var buf bytes.Buffer
	if err := writeMerged(&buf, "", map[statementKey]statementRecord{}); err != nil {
		t.Fatalf("writeMerged: %v", err)
	}
	if !strings.Contains(buf.String(), "mode: atomic") {
		t.Fatalf("empty merge should still emit a mode header; got %q", buf.String())
	}
}

func TestWriteMerged_OrderIsDeterministic(t *testing.T) {
	records := map[statementKey]statementRecord{
		{file: "b.go", block: "1.0,2.0"}: {numStmt: 1, count: 1},
		{file: "a.go", block: "2.0,3.0"}: {numStmt: 1, count: 1},
		{file: "a.go", block: "1.0,2.0"}: {numStmt: 1, count: 1},
	}
	var first bytes.Buffer
	if err := writeMerged(&first, "atomic", records); err != nil {
		t.Fatalf("writeMerged: %v", err)
	}
	var second bytes.Buffer
	if err := writeMerged(&second, "atomic", records); err != nil {
		t.Fatalf("writeMerged: %v", err)
	}
	if first.String() != second.String() {
		t.Fatalf("non-deterministic order:\n--first--\n%s\n--second--\n%s", first.String(), second.String())
	}
	// Check a.go sorts before b.go and within a.go the blocks sort lexically.
	got := first.String()
	idxA12 := strings.Index(got, "a.go:1.0,2.0")
	idxA23 := strings.Index(got, "a.go:2.0,3.0")
	idxB := strings.Index(got, "b.go:1.0,2.0")
	if idxA12 < 0 || idxA23 < 0 || idxB < 0 {
		t.Fatalf("missing expected lines:\n%s", got)
	}
	if !(idxA12 < idxA23 && idxA23 < idxB) {
		t.Fatalf("wrong order; got:\n%s", got)
	}
}
