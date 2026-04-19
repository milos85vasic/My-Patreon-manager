// Package e2e_test contains end-to-end tests that stitch the process
// pipeline together with a real migrated SQLite database and an
// httptest-backed Patreon server. These replace the legacy orchestrator
// end-to-end tests removed in commit 0c46639.
package e2e_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/patreon"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/process"
	"github.com/milos85vasic/My-Patreon-Manager/internal/testhelpers"
)

// stubArticleGenerator satisfies process.ArticleGenerator with a fixed
// title/body pair.
type stubArticleGenerator struct {
	title string
	body  string
}

func (s *stubArticleGenerator) Generate(_ context.Context, _ *models.Repository) (string, string, error) {
	return s.title, s.body, nil
}

// patreonMutatorE2E wraps a real *patreon.Client so it satisfies
// process.PatreonMutator. It mirrors cmd/cli/publish.go's
// patreonMutatorAdapter — the real production adapter — so the E2E test
// exercises the same code path the CLI would use at runtime.
type patreonMutatorE2E struct {
	c *patreon.Client
}

func (a *patreonMutatorE2E) GetPostContent(ctx context.Context, postID string) (string, error) {
	post, err := a.c.GetPost(ctx, postID)
	if err != nil {
		return "", err
	}
	if post == nil {
		return "", nil
	}
	return post.Content, nil
}

func (a *patreonMutatorE2E) CreatePost(ctx context.Context, title, body string, _ *string) (string, error) {
	created, err := a.c.CreatePost(ctx, &models.Post{
		Title:    title,
		Content:  body,
		PostType: "text_only",
	})
	if err != nil {
		return "", err
	}
	return created.ID, nil
}

func (a *patreonMutatorE2E) UpdatePost(ctx context.Context, postID, title, body string, _ *string) error {
	_, err := a.c.UpdatePost(ctx, &models.Post{
		ID:       postID,
		Title:    title,
		Content:  body,
		PostType: "text_only",
	})
	return err
}

// TestE2E_ProcessApprovePublish walks the full pipeline:
//  1. Seed a repository row in a fresh migrated SQLite DB.
//  2. Run process.Pipeline.ProcessRepo with a stubbed article generator
//     to land a new pending_review revision.
//  3. Flip the revision to "approved" (simulating the preview-UI action).
//  4. Spin an httptest.Server that acts as the Patreon API.
//  5. Publish via a real *patreon.Client wrapped in the production-style
//     PatreonMutator adapter.
//  6. Assert bookkeeping: revision.patreon_post_id set, repo.
//     published_revision_id points to the published revision.
func TestE2E_ProcessApprovePublish(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	// --- Step 1: Seed the repo ---
	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO repositories (id, service, owner, name, url, https_url, last_commit_sha)
		 VALUES ('r1', 'github', 'acme', 'demo', 'u', 'h', 'sha1')`); err != nil {
		t.Fatalf("seed repo: %v", err)
	}

	// --- Steps 2-4: Run the pipeline ---
	gen := &stubArticleGenerator{title: "Demo v1", body: "body for r1"}
	pipe := process.NewPipeline(process.PipelineDeps{
		DB:               db,
		Generator:        gen,
		IllustrationGen:  nil, // intentionally no illustrations
		GeneratorVersion: "v1",
	})
	if err := pipe.ProcessRepo(ctx, "r1"); err != nil {
		t.Fatalf("pipeline: %v", err)
	}

	// Verify exactly one pending_review revision landed.
	pending, err := db.ContentRevisions().ListByRepoStatus(ctx, "r1", models.RevisionStatusPendingReview)
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("want 1 pending_review, got %d", len(pending))
	}
	rev := pending[0]

	// --- Step 5: Approve ---
	if err := db.ContentRevisions().UpdateStatus(ctx, rev.ID, models.RevisionStatusApproved); err != nil {
		t.Fatalf("approve: %v", err)
	}

	// --- Step 6: Fake Patreon server ---
	postCreateCalled := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The CreatePost path is POST /posts; nothing else should be
		// hit on a first-publish flow.
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/posts") {
			postCreateCalled++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]interface{}{
					"id":   "PPNEW",
					"type": "post",
					"attributes": map[string]interface{}{
						"title":   "Demo v1",
						"content": "body for r1",
					},
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// --- Step 7: Real patreon.Client against the fake server ---
	oauth := patreon.NewOAuth2Manager("cid", "cs", "access", "refresh")
	client := patreon.NewClient(oauth, "camp1")
	client.SetBaseURL(server.URL)

	// --- Step 8: Publish ---
	pub := process.NewPublisher(db, &patreonMutatorE2E{c: client})
	n, err := pub.PublishPending(ctx)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if n != 1 {
		t.Fatalf("want 1 published, got %d", n)
	}
	if postCreateCalled != 1 {
		t.Fatalf("want 1 CreatePost call, got %d", postCreateCalled)
	}

	// The revision row must carry the Patreon post id returned by the
	// fake server.
	rev2, err := db.ContentRevisions().GetByID(ctx, rev.ID)
	if err != nil {
		t.Fatalf("reload revision: %v", err)
	}
	if rev2 == nil {
		t.Fatalf("revision disappeared after publish")
	}
	if rev2.PatreonPostID == nil || *rev2.PatreonPostID != "PPNEW" {
		t.Fatalf("revision patreon_post_id: %v", rev2.PatreonPostID)
	}
	if rev2.PublishedToPatreonAt == nil {
		t.Fatalf("revision published_to_patreon_at not stamped")
	}

	// The repository row must now point at the published revision.
	repo, err := db.Repositories().GetByID(ctx, "r1")
	if err != nil {
		t.Fatalf("reload repo: %v", err)
	}
	if repo == nil {
		t.Fatalf("repo disappeared after publish")
	}
	if repo.PublishedRevisionID == nil || *repo.PublishedRevisionID != rev.ID {
		t.Fatalf("repo published_revision_id: %v", repo.PublishedRevisionID)
	}
}
