package process

import (
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

// These white-box tests cover the defensive edge paths of the
// layered matchers (empty inputs, nil pointers, malformed rows,
// regex oddities). The main black-box coverage lives in
// import_test.go; this file pins the cheap guard branches that the
// integration tests don't naturally exercise.

func TestMatchByTag_EmptyContent(t *testing.T) {
	if r := matchByTag(PatreonPost{Title: "x"}, []*models.Repository{{ID: "r1"}}); r != nil {
		t.Fatalf("empty content should not match: %v", r)
	}
}

func TestMatchByTag_SkipsNilAndEmptyID(t *testing.T) {
	repos := []*models.Repository{
		nil,
		{ID: ""},
		{ID: "r1"},
	}
	got := matchByTag(PatreonPost{Content: "see repo:r1 for details"}, repos)
	if got == nil || got.ID != "r1" {
		t.Fatalf("expected r1, got %+v", got)
	}
}

func TestMatchByURL_EmptyContent(t *testing.T) {
	repos := []*models.Repository{{URL: "https://example.com/foo"}}
	if r := matchByURL(PatreonPost{}, repos); r != nil {
		t.Fatalf("empty content should not match: %v", r)
	}
}

func TestMatchByURL_SkipsNilAndShortURLs(t *testing.T) {
	repos := []*models.Repository{
		nil,
		{URL: "u", HTTPSURL: "h"},                          // both too short — skipped
		{URL: "", HTTPSURL: "https://example.com/project"}, // matches
	}
	got := matchByURL(PatreonPost{Content: "visit https://example.com/project now"}, repos)
	if got == nil || got.HTTPSURL != "https://example.com/project" {
		t.Fatalf("expected https://example.com/project match, got %+v", got)
	}
}

func TestMatchBySlug_EmptyTitle(t *testing.T) {
	repos := []*models.Repository{{Name: "r"}}
	if r := matchBySlug(PatreonPost{}, repos); r != nil {
		t.Fatalf("empty title should not match: %v", r)
	}
}

func TestMatchBySlug_SkipsNilAndEmptyName(t *testing.T) {
	repos := []*models.Repository{
		nil,
		{Name: ""},
		{Owner: "acme", Name: "widget"},
	}
	got := matchBySlug(PatreonPost{Title: "acme/widget update"}, repos)
	if got == nil || got.Name != "widget" {
		t.Fatalf("expected widget match, got %+v", got)
	}
}

func TestMatchBySubstring_EmptyTitle(t *testing.T) {
	if r := matchBySubstring(PatreonPost{}, []*models.Repository{{Name: "x"}}); r != nil {
		t.Fatalf("empty title should not match: %v", r)
	}
}

func TestMatchBySubstring_SkipsNilAndEmptyName(t *testing.T) {
	repos := []*models.Repository{
		nil,
		{Name: ""},
		{Name: "widget"},
	}
	got := matchBySubstring(PatreonPost{Title: "the widget release"}, repos)
	if got == nil || got.Name != "widget" {
		t.Fatalf("expected widget match, got %+v", got)
	}
}

func TestContainsWholeWord_EmptyNeedle(t *testing.T) {
	if containsWholeWord("any haystack", "") {
		t.Fatal("empty needle must never match")
	}
}

func TestContainsWholeWord_RegexMetacharactersEscaped(t *testing.T) {
	// Repo names with regex-meaningful characters are common. The
	// QuoteMeta call in containsWholeWord must make them literal.
	if !containsWholeWord("release my.repo.v1 tomorrow", "my.repo") {
		t.Fatal("QuoteMeta should allow literal dot match")
	}
	if containsWholeWord("release myxrepo tomorrow", "my.repo") {
		t.Fatal("literal dot must NOT match arbitrary char")
	}
}

func TestNormalizeURL(t *testing.T) {
	cases := map[string]string{
		"":                                 "",
		"   ":                              "",
		"HTTPS://Example.COM/Foo/":         "https://example.com/foo",
		"https://example.com/foo//":        "https://example.com/foo",
		"git@github.com:acme/widget.git/ ": "git@github.com:acme/widget.git",
	}
	for in, want := range cases {
		if got := normalizeURL(in); got != want {
			t.Errorf("normalizeURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLooksLikeURL(t *testing.T) {
	cases := map[string]bool{
		"":                        false,
		"u":                        false,
		"short!!":                  false,
		"https://example.com/foo":  true,
		"git@github.com:acme/widget.git": true,
		"plain string no sep long": false,
	}
	for in, want := range cases {
		if got := looksLikeURL(in); got != want {
			t.Errorf("looksLikeURL(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestMatchRepoLayered_ReturnsNil(t *testing.T) {
	// All layers miss — returns nil.
	if r := matchRepoLayered(PatreonPost{Title: "x", Content: "y"}, nil); r != nil {
		t.Fatalf("expected nil, got %+v", r)
	}
}
