package challenges

import (
	"context"
	"strings"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/services/filter"
	"github.com/milos85vasic/My-Patreon-Manager/internal/utils"
)

// TestChallenges_All drives every registered project scenario through the
// shared runner so a single `go test ./tests/challenges/` invocation
// reports on the whole suite. New scenarios only need to add themselves
// to registerScenarios() — no extra Test boilerplate per case.
func TestChallenges_All(t *testing.T) {
	reg := NewRegistry()
	registerScenarios(reg)
	reg.RunAll(t)
}

// registerScenarios is the central registry of project-specific
// scenarios. Each entry captures one operator-facing workflow or
// invariant that spans multiple internal packages — use it to catch
// regressions that per-package unit tests can't see.
func registerScenarios(r *Registry) {
	r.MustRegister(Scenario{
		Name:        "redact-url-strips-userinfo",
		Description: "Any URL with a user:password@ authority must have its credentials replaced end-to-end by utils.RedactURL.",
		Execute: func(_ context.Context) (*Report, error) {
			inputs := []string{
				"https://user:pass@github.com/owner/repo",
				"https://token:@gitlab.com/owner/repo?page=2",
				"ssh://gitflic-user:xyz@gitflic.ru:2222/owner/repo.git",
				"https://:only-password@gitverse.ru/owner/repo",
			}
			outs := make([]string, len(inputs))
			for i, in := range inputs {
				outs[i] = utils.RedactURL(in)
			}
			return &Report{Outputs: map[string]any{
				"inputs":  inputs,
				"outputs": outs,
			}}, nil
		},
		Assert: func(t *testing.T, r *Report) {
			ins, _ := r.Outputs["inputs"].([]string)
			outs, _ := r.Outputs["outputs"].([]string)
			for i := range ins {
				// No raw `user:pass@` combination survives. The
				// redactor substitutes ***:***@ for every userinfo
				// segment we feed it.
				if strings.Contains(outs[i], "user:pass@") ||
					strings.Contains(outs[i], "token:@") ||
					strings.Contains(outs[i], "gitflic-user:xyz@") ||
					strings.Contains(outs[i], ":only-password@") {
					t.Fatalf("scenario %s: credential leaked: %q → %q", r.Scenario, ins[i], outs[i])
				}
				if !strings.Contains(outs[i], "***:***@") && !strings.Contains(outs[i], "?***") {
					t.Fatalf("scenario %s: expected redaction sentinel in %q", r.Scenario, outs[i])
				}
			}
		},
	})

	r.MustRegister(Scenario{
		Name:        "repoignore-matches-default-patterns",
		Description: "Default (empty-file) repoignore must match nothing — operators rely on this to avoid accidental exclusions before they author a .repoignore.",
		Execute: func(_ context.Context) (*Report, error) {
			rep, err := filter.ParseRepoignoreFile("")
			if err != nil {
				return nil, err
			}
			urls := []string{
				"https://github.com/owner/repo",
				"git@gitlab.com:owner/repo.git",
				"",
				"https://gitflic.ru/owner/repo-with-special!@#$-chars",
			}
			results := make(map[string]bool, len(urls))
			for _, u := range urls {
				results[u] = rep.Match(u)
			}
			return &Report{Outputs: map[string]any{"matches": results}}, nil
		},
		Assert: func(t *testing.T, r *Report) {
			matches, _ := r.Outputs["matches"].(map[string]bool)
			for url, matched := range matches {
				if matched {
					t.Fatalf("scenario %s: empty repoignore should not match %q", r.Scenario, url)
				}
			}
		},
	})

	r.MustRegister(Scenario{
		Name:        "normalize-https-is-idempotent",
		Description: "NormalizeHTTPS applied twice must return the same result as applied once — log rotation, dedup, and hash-keyed storage all rely on this.",
		Execute: func(_ context.Context) (*Report, error) {
			inputs := []string{
				"git@github.com:owner/repo.git",
				"https://github.com/owner/repo/",
				"https://GitLab.com/owner/repo",
				"ssh://git@gitflic.ru:2222/owner/repo.git",
				"",
			}
			pass1 := make([]string, len(inputs))
			pass2 := make([]string, len(inputs))
			for i, in := range inputs {
				pass1[i] = utils.NormalizeHTTPS(in)
				pass2[i] = utils.NormalizeHTTPS(pass1[i])
			}
			return &Report{Outputs: map[string]any{
				"inputs": inputs,
				"pass1":  pass1,
				"pass2":  pass2,
			}}, nil
		},
		Assert: func(t *testing.T, r *Report) {
			ins, _ := r.Outputs["inputs"].([]string)
			p1, _ := r.Outputs["pass1"].([]string)
			p2, _ := r.Outputs["pass2"].([]string)
			for i := range ins {
				if p1[i] != p2[i] {
					t.Fatalf("scenario %s: %q → %q (pass1) → %q (pass2) — not idempotent", r.Scenario, ins[i], p1[i], p2[i])
				}
			}
		},
	})
}

// TestRegistry_RejectsDuplicates, TestRegistry_RejectsMalformed, and
// TestRegistry_RunIsolation exercise the harness itself so regressions
// in the scenario plumbing surface as test failures rather than
// hiding behind silently-not-running scenarios.

func TestRegistry_RejectsDuplicates(t *testing.T) {
	reg := NewRegistry()
	s := Scenario{
		Name:    "dup",
		Execute: func(context.Context) (*Report, error) { return &Report{}, nil },
		Assert:  func(*testing.T, *Report) {},
	}
	if err := reg.Register(s); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := reg.Register(s); err == nil {
		t.Fatal("duplicate Register should error")
	}
}

func TestRegistry_RejectsMalformed(t *testing.T) {
	reg := NewRegistry()
	cases := []Scenario{
		{Name: "", Execute: func(context.Context) (*Report, error) { return &Report{}, nil }, Assert: func(*testing.T, *Report) {}},
		{Name: "n", Execute: nil, Assert: func(*testing.T, *Report) {}},
		{Name: "n", Execute: func(context.Context) (*Report, error) { return &Report{}, nil }, Assert: nil},
	}
	for i, c := range cases {
		if err := reg.Register(c); err == nil {
			t.Fatalf("case %d: malformed scenario should error", i)
		}
	}
}

func TestRegistry_RunPopulatesReportMetadata(t *testing.T) {
	reg := NewRegistry()
	var seen *Report
	reg.MustRegister(Scenario{
		Name:    "metadata",
		Execute: func(context.Context) (*Report, error) { return &Report{Outputs: map[string]any{}}, nil },
		Assert: func(_ *testing.T, r *Report) {
			seen = r
		},
	})
	reg.RunAll(t)
	if seen == nil {
		t.Fatal("scenario did not run")
	}
	if seen.Scenario != "metadata" {
		t.Fatalf("Scenario field not populated: %+v", seen)
	}
	if seen.Started.IsZero() {
		t.Fatalf("Started not populated: %+v", seen)
	}
	if seen.Duration <= 0 {
		t.Fatalf("Duration not populated: %+v", seen)
	}
}
