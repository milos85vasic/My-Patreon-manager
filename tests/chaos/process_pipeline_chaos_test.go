// Package chaos_test contains resilience tests for the process pipeline
// and publisher under partial upstream failures. They replace the legacy
// orchestrator chaos tests removed in commit 0c46639.
package chaos_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/patreon"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/process"
	"github.com/milos85vasic/My-Patreon-Manager/internal/testhelpers"
)

// chaoticGenerator returns an LLM-style transient failure on a
// configurable fraction of calls. The deterministic *rand.Rand makes
// test runs reproducible.
type chaoticGenerator struct {
	failRate float64
	rng      *rand.Rand
}

func (c *chaoticGenerator) Generate(_ context.Context, repo *models.Repository) (string, string, error) {
	if c.rng.Float64() < c.failRate {
		return "", "", errors.New("LLM rate limit")
	}
	return "T-" + repo.Name, "body for " + repo.Name, nil
}

// seedChaosRepo inserts a repositories row with a unique last_commit_sha
// (so fingerprints differ across repos and nothing dedup-collides).
func seedChaosRepo(t *testing.T, db *database.SQLiteDB, id string) {
	t.Helper()
	_, err := db.DB().ExecContext(context.Background(),
		`INSERT INTO repositories (id, service, owner, name, url, https_url, last_commit_sha, process_state)
		 VALUES (?, 'github', 'o', ?, 'u', 'h', ?, 'idle')`,
		id, id, "sha-"+id)
	if err != nil {
		t.Fatalf("seed repo %s: %v", id, err)
	}
}

// TestChaos_FlakyGenerator runs the pipeline across 10 repos with a
// generator that fails ~30 % of the time. The pipeline must:
//   - complete (no hangs, no panics);
//   - never leave a repo stuck in process_state='processing';
//   - land exactly `success` pending_review revisions in the DB
//     (no ghost rows from aborted attempts).
func TestChaos_FlakyGenerator(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	const repoCount = 10
	for i := 0; i < repoCount; i++ {
		seedChaosRepo(t, db, fmt.Sprintf("r%d", i))
	}

	// Deterministic seed -> deterministic split between success and
	// failure across this fixed 10-repo workload.
	gen := &chaoticGenerator{failRate: 0.3, rng: rand.New(rand.NewSource(1))}
	pipe := process.NewPipeline(process.PipelineDeps{
		DB:               db,
		Generator:        gen,
		GeneratorVersion: "v1",
	})

	success, failed := 0, 0
	for i := 0; i < repoCount; i++ {
		id := fmt.Sprintf("r%d", i)
		err := pipe.ProcessRepo(ctx, id)
		if err == nil {
			success++
		} else {
			failed++
		}

		// Whatever happened, the repo must not be stuck mid-flight.
		// Accept "idle" (success and reverted-failure both land here —
		// success flips to awaiting_review immediately below, but reverts
		// leave it at 'idle') or "awaiting_review" on success.
		repo, gerr := db.Repositories().GetByID(ctx, id)
		if gerr != nil {
			t.Fatalf("reload %s: %v", id, gerr)
		}
		if repo == nil {
			t.Fatalf("repo %s vanished", id)
		}
		if repo.ProcessState == "processing" {
			t.Fatalf("%s stuck in processing after ProcessRepo", id)
		}
		if err != nil && repo.ProcessState != "idle" {
			t.Fatalf("%s failed but state is %q (want 'idle')", id, repo.ProcessState)
		}
		if err == nil && repo.ProcessState != "awaiting_review" {
			t.Fatalf("%s succeeded but state is %q (want 'awaiting_review')", id, repo.ProcessState)
		}
	}
	if success+failed != repoCount {
		t.Fatalf("accounting: success=%d failed=%d (want total=%d)", success, failed, repoCount)
	}
	if success == 0 || failed == 0 {
		t.Fatalf("chaos split degenerate: success=%d failed=%d; reseed", success, failed)
	}

	// The pending_review count must match successful generations —
	// failed calls must not leave half-inserted revisions.
	var pending int
	if err := db.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM content_revisions WHERE status = 'pending_review'`).Scan(&pending); err != nil {
		t.Fatalf("count pending: %v", err)
	}
	if pending != success {
		t.Fatalf("pending_review count = %d, want %d", pending, success)
	}
}

// flakyPatreonServer returns an httptest.Server whose POST /posts
// handler fails with 500 roughly 50 % of the time (driven by a
// configurable oracle so tests stay deterministic). Successful calls
// return a unique post id on each call.
func flakyPatreonServer(shouldFail func(callIdx int) bool) (*httptest.Server, *int32) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/posts")) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		idx := int(atomic.AddInt32(&calls, 1) - 1)
		if shouldFail(idx) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errors":[{"detail":"transient"}]}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"id":   fmt.Sprintf("PP%d", idx),
				"type": "post",
				"attributes": map[string]interface{}{
					"title":   "t",
					"content": "b",
				},
			},
		})
	}))
	return srv, &calls
}

// patreonMutatorChaos is a tiny adapter onto *patreon.Client — identical
// in shape to cmd/cli/publish.go's patreonMutatorAdapter. A copy lives
// here rather than being shared so each chaos scenario keeps the
// plumbing visible in one place.
type patreonMutatorChaos struct {
	c *patreon.Client
}

func (a *patreonMutatorChaos) GetPostContent(ctx context.Context, postID string) (string, error) {
	post, err := a.c.GetPost(ctx, postID)
	if err != nil {
		return "", err
	}
	if post == nil {
		return "", nil
	}
	return post.Content, nil
}

func (a *patreonMutatorChaos) CreatePost(ctx context.Context, title, body string, _ *string) (string, error) {
	created, err := a.c.CreatePost(ctx, &models.Post{Title: title, Content: body, PostType: "text_only"})
	if err != nil {
		return "", err
	}
	return created.ID, nil
}

func (a *patreonMutatorChaos) UpdatePost(ctx context.Context, postID, title, body string, _ *string) error {
	_, err := a.c.UpdatePost(ctx, &models.Post{ID: postID, Title: title, Content: body, PostType: "text_only"})
	return err
}

// TestChaos_FlakyPatreonAPI seeds 8 repos, each with an approved
// revision ready to publish, and points the publisher at a Patreon
// server that 500s on the first of every pair of calls. It asserts:
//   - failed repos do not block successful ones (per-repo tolerance);
//   - no half-published revisions (patreon_post_id and
//     published_to_patreon_at are set together or not at all);
//   - a second publish pass against a healthy server heals every
//     failed repo so the whole fleet ends up published.
func TestChaos_FlakyPatreonAPI(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	const repoCount = 8
	type seeded struct {
		repoID string
		revID  string
	}
	seeds := make([]seeded, 0, repoCount)

	// Seed repos + an approved revision on each. The pipeline could do
	// this too, but driving the SQL directly keeps this test focused on
	// the publisher's resilience rather than the full round-trip.
	for i := 0; i < repoCount; i++ {
		rid := fmt.Sprintf("cr%d", i)
		seedChaosRepo(t, db, rid)

		revID := fmt.Sprintf("rev-%d", i)
		rev := &models.ContentRevision{
			ID:               revID,
			RepositoryID:     rid,
			Version:          1,
			Source:           "generated",
			Status:           models.RevisionStatusApproved,
			Title:            fmt.Sprintf("T%d", i),
			Body:             fmt.Sprintf("body %d", i),
			Fingerprint:      fmt.Sprintf("fp-%d", i),
			GeneratorVersion: "v1",
			SourceCommitSHA:  "sha-" + rid,
			Author:           "system",
		}
		if err := db.ContentRevisions().Create(ctx, rev); err != nil {
			t.Fatalf("create revision %s: %v", revID, err)
		}
		seeds = append(seeds, seeded{repoID: rid, revID: revID})
	}

	// Half of the POST /posts calls return 500. Deterministic: the
	// 0th, 2nd, 4th, ... calls fail; the rest succeed.
	shouldFail := func(idx int) bool { return idx%2 == 0 }
	srv, _ := flakyPatreonServer(shouldFail)
	defer srv.Close()

	oauth := patreon.NewOAuth2Manager("cid", "cs", "access", "refresh")
	client := patreon.NewClient(oauth, "camp1")
	client.SetBaseURL(srv.URL)
	// Shrink the retry budget so this test stays fast; the
	// circuit-breaker + backoff math on the default 3 retries would
	// otherwise dominate wall-clock time on a flaky workload.
	client.SetMaxRetries(1)

	pub := process.NewPublisher(db, &patreonMutatorChaos{c: client})
	firstCount, err := pub.PublishPending(ctx)
	if err != nil {
		t.Fatalf("publish pass 1: %v", err)
	}
	if firstCount <= 0 || firstCount >= repoCount {
		t.Fatalf("pass 1 expected partial success, got %d/%d", firstCount, repoCount)
	}

	// Every revision must be either fully published (both markers set)
	// or fully untouched (both markers nil). No half-published rows.
	for _, s := range seeds {
		rev, err := db.ContentRevisions().GetByID(ctx, s.revID)
		if err != nil {
			t.Fatalf("reload %s: %v", s.revID, err)
		}
		if rev == nil {
			t.Fatalf("revision %s vanished", s.revID)
		}
		idSet := rev.PatreonPostID != nil && *rev.PatreonPostID != ""
		tsSet := rev.PublishedToPatreonAt != nil
		if idSet != tsSet {
			t.Fatalf("half-published revision %s: patreon_post_id=%v published_to_patreon_at=%v",
				s.revID, rev.PatreonPostID, rev.PublishedToPatreonAt)
		}
	}

	// --- Retry pass against a healthy server ---
	// Point the client at a server that always succeeds, then re-run
	// PublishPending. Every previously-failed repo must now be
	// published, and the total cross both runs must equal repoCount.
	healthy, _ := flakyPatreonServer(func(int) bool { return false })
	defer healthy.Close()
	client.SetBaseURL(healthy.URL)

	secondCount, err := pub.PublishPending(ctx)
	if err != nil {
		t.Fatalf("publish pass 2: %v", err)
	}
	if firstCount+secondCount != repoCount {
		t.Fatalf("healed count: first=%d second=%d want total=%d", firstCount, secondCount, repoCount)
	}

	// After the healing pass, every repo's newest revision must carry a
	// Patreon post id.
	for _, s := range seeds {
		rev, err := db.ContentRevisions().GetByID(ctx, s.revID)
		if err != nil {
			t.Fatalf("reload %s: %v", s.revID, err)
		}
		if rev.PatreonPostID == nil || *rev.PatreonPostID == "" {
			t.Fatalf("revision %s never published after retry", s.revID)
		}
		if rev.PublishedToPatreonAt == nil {
			t.Fatalf("revision %s missing published_to_patreon_at after retry", s.revID)
		}
	}

	// No repo should be stuck in 'processing' (the publisher shouldn't
	// set that state, but assert anyway to catch future regressions).
	for _, s := range seeds {
		repo, err := db.Repositories().GetByID(ctx, s.repoID)
		if err != nil {
			t.Fatalf("reload repo %s: %v", s.repoID, err)
		}
		if repo.ProcessState == "processing" {
			t.Fatalf("repo %s stuck in processing", s.repoID)
		}
	}
}
