// Package stress exercises the project's hot paths with large inputs
// to catch algorithmic regressions (unintended quadratic behavior,
// unbounded allocations, forgotten pagination) before they reach
// production-scale data.
//
// Stress tests live in their own package so they can be opt-in via
// `go test -short=false ./tests/stress/...` — every test here honors
// `testing.Short()` and t.Skip so `go test -short ./...` keeps the
// default test run fast.
package stress

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/git"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/filter"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/sync"
	"github.com/milos85vasic/My-Patreon-Manager/internal/utils"
)

// makeRepos constructs n pseudo-repositories fanned out across the four
// supported Git services. Owner and name patterns are deterministic so
// repeated runs compare like-for-like.
func makeRepos(n int) []models.Repository {
	services := []string{"github", "gitlab", "gitflic", "gitverse"}
	repos := make([]models.Repository, n)
	for i := 0; i < n; i++ {
		svc := services[i%len(services)]
		owner := fmt.Sprintf("owner-%d", i%50)
		name := fmt.Sprintf("repo-%d", i)
		repos[i] = models.Repository{
			ID:       fmt.Sprintf("%s-%s-%s", svc, owner, name),
			Service:  svc,
			Owner:    owner,
			Name:     name,
			HTTPSURL: fmt.Sprintf("https://%s.com/%s/%s", svc, owner, name),
			URL:      fmt.Sprintf("git@%s.com:%s/%s.git", svc, owner, name),
		}
	}
	return repos
}

// TestStress_ApplyFilter_TenThousandRepos drives the sync filter across
// a 10k-repo portfolio and enforces a wall-clock budget to catch
// O(n²) or accidentally nested-loop regressions.
func TestStress_ApplyFilter_TenThousandRepos(t *testing.T) {
	if testing.Short() {
		t.Skip("stress test skipped under -short")
	}
	repos := makeRepos(10_000)
	start := time.Now()
	for i := 0; i < 5; i++ {
		out := sync.ApplyFilter(repos, sync.SyncFilter{Org: "owner-7"}, nil)
		if len(out) == 0 {
			t.Fatalf("ApplyFilter returned zero repos for a matching org")
		}
	}
	elapsed := time.Since(start)
	// Budget: 5 invocations across 10k repos should finish in under 2s
	// on every reasonable dev machine. The real filter is linear, so
	// even a 5-10× slowdown triggers.
	if elapsed > 2*time.Second {
		t.Fatalf("ApplyFilter×5 on 10k repos took %v; budget 2s — likely regression", elapsed)
	}
}

// TestStress_DetectMirrors_FiveThousandRepos exercises the mirror
// detector (README hash + commit SHA comparison) against a 5k portfolio
// where every repo has three mirror candidates across other services.
func TestStress_DetectMirrors_FiveThousandRepos(t *testing.T) {
	if testing.Short() {
		t.Skip("stress test skipped under -short")
	}
	repos := makeRepos(5_000)
	start := time.Now()
	mirrors, err := git.DetectMirrors(context.Background(), repos)
	if err != nil {
		t.Fatalf("DetectMirrors: %v", err)
	}
	elapsed := time.Since(start)
	// Budget picked to accommodate `-race` detector overhead (observed
	// ~24s under race on a healthy laptop, ~16s without). 60s leaves
	// headroom for loaded CI and catches real regressions that would
	// push this into minutes.
	if elapsed > 60*time.Second {
		t.Fatalf("DetectMirrors on 5k repos took %v; budget 60s — likely regression", elapsed)
	}
	t.Logf("detected %d mirror groups in %v", len(mirrors), elapsed)
}

// TestStress_RepoignoreMatch_OneMillionCalls asserts the .repoignore
// matcher is fast enough to stay off the critical path. Current impl
// is trie-based; a regression to naive regex iteration would show up
// here as a ~100× slowdown.
func TestStress_RepoignoreMatch_OneMillionCalls(t *testing.T) {
	if testing.Short() {
		t.Skip("stress test skipped under -short")
	}
	rep, err := filter.ParseRepoignoreFile("")
	if err != nil {
		t.Fatalf("ParseRepoignoreFile: %v", err)
	}

	url := "https://github.com/owner-7/repo-42"
	start := time.Now()
	for i := 0; i < 1_000_000; i++ {
		rep.Match(url)
	}
	elapsed := time.Since(start)
	if elapsed > 5*time.Second {
		t.Fatalf("1M Match() calls took %v; budget 5s — likely regression", elapsed)
	}
}

// TestStress_NormalizeHTTPS_HundredThousandUnique exercises the URL
// normalizer with a mix of SSH, HTTPS, trailing-slash, and
// uppercase-host inputs to catch allocation regressions.
func TestStress_NormalizeHTTPS_HundredThousandUnique(t *testing.T) {
	if testing.Short() {
		t.Skip("stress test skipped under -short")
	}
	inputs := make([]string, 100_000)
	for i := range inputs {
		switch i % 4 {
		case 0:
			inputs[i] = fmt.Sprintf("git@github.com:owner-%d/repo-%d.git", i%100, i)
		case 1:
			inputs[i] = fmt.Sprintf("https://github.com/owner-%d/repo-%d", i%100, i)
		case 2:
			inputs[i] = fmt.Sprintf("https://GitLab.com/owner-%d/repo-%d/", i%100, i)
		case 3:
			inputs[i] = fmt.Sprintf("ssh://git@gitflic.ru:2222/owner-%d/repo-%d.git", i%100, i)
		}
	}

	start := time.Now()
	for _, in := range inputs {
		_ = utils.NormalizeHTTPS(in)
	}
	elapsed := time.Since(start)
	// Budget picked to accommodate `-race` detector overhead (observed
	// ~5.5s under race on a healthy laptop, ~1s without). 10s still
	// catches regressions that push throughput below ~10k calls/s.
	if elapsed > 10*time.Second {
		t.Fatalf("100k NormalizeHTTPS calls took %v; budget 10s — likely regression", elapsed)
	}
}
