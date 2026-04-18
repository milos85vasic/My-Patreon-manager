package database_test

import (
	"context"
	"testing"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/testhelpers"
)

func TestUnmatched_Record_And_List(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	_ = db.UnmatchedPatreonPosts().Record(ctx, &models.UnmatchedPatreonPost{
		ID: "u1", PatreonPostID: "p1", Title: "Hello", URL: "http://x/1",
		RawPayload: `{"p":"1"}`,
	})
	time.Sleep(2 * time.Millisecond) // ensure ORDER BY discovered_at ASC is deterministic
	_ = db.UnmatchedPatreonPosts().Record(ctx, &models.UnmatchedPatreonPost{
		ID: "u2", PatreonPostID: "p2", Title: "World", URL: "http://x/2",
		RawPayload: `{"p":"2"}`,
	})

	list, err := db.UnmatchedPatreonPosts().ListPending(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2, got %d", len(list))
	}
	if list[0].PatreonPostID != "p1" || list[1].PatreonPostID != "p2" {
		t.Fatalf("wrong order: %+v", list)
	}
}

func TestUnmatched_Record_IdempotentOnPatreonPostID(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	err := db.UnmatchedPatreonPosts().Record(ctx, &models.UnmatchedPatreonPost{
		ID: "u1", PatreonPostID: "p", Title: "T1", URL: "http://x/1", RawPayload: "{}",
	})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	err = db.UnmatchedPatreonPosts().Record(ctx, &models.UnmatchedPatreonPost{
		ID: "u2", PatreonPostID: "p", Title: "T2", URL: "http://x/2", RawPayload: "{}",
	})
	if err != nil {
		t.Fatalf("dup should be no-op, got: %v", err)
	}

	list, _ := db.UnmatchedPatreonPosts().ListPending(ctx)
	if len(list) != 1 {
		t.Fatalf("want 1 after dedup, got %d", len(list))
	}
	if list[0].ID != "u1" || list[0].Title != "T1" {
		t.Fatalf("should keep original row: %+v", list[0])
	}
}

func TestUnmatched_Resolve(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	_, _ = db.DB().ExecContext(ctx,
		`INSERT INTO repositories (id, service, owner, name, url, https_url) VALUES ('r1','github','o','n','u','h')`)
	_ = db.UnmatchedPatreonPosts().Record(ctx, &models.UnmatchedPatreonPost{
		ID: "u1", PatreonPostID: "p1", Title: "T", URL: "http://x", RawPayload: "{}",
	})
	if err := db.UnmatchedPatreonPosts().Resolve(ctx, "u1", "r1"); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	list, _ := db.UnmatchedPatreonPosts().ListPending(ctx)
	if len(list) != 0 {
		t.Fatalf("resolved row still listed: %+v", list)
	}
}

func TestUnmatched_Resolve_NotFound(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	err := db.UnmatchedPatreonPosts().Resolve(context.Background(), "nope", "r1")
	if err == nil {
		t.Fatal("expected error on missing id")
	}
}

func TestUnmatched_Resolve_AlreadyResolved(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	_, _ = db.DB().ExecContext(ctx,
		`INSERT INTO repositories (id, service, owner, name, url, https_url) VALUES ('r1','github','o','n','u','h')`)
	_ = db.UnmatchedPatreonPosts().Record(ctx, &models.UnmatchedPatreonPost{
		ID: "u1", PatreonPostID: "p1", Title: "T", URL: "http://x", RawPayload: "{}",
	})
	if err := db.UnmatchedPatreonPosts().Resolve(ctx, "u1", "r1"); err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	err := db.UnmatchedPatreonPosts().Resolve(ctx, "u1", "r1")
	if err == nil {
		t.Fatal("expected error on double-resolve")
	}
}

func TestUnmatched_Record_WithPublishedAt(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	pub := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	_ = db.UnmatchedPatreonPosts().Record(ctx, &models.UnmatchedPatreonPost{
		ID: "u1", PatreonPostID: "p", Title: "T", URL: "http://x",
		PublishedAt: &pub, RawPayload: "{}",
	})
	list, _ := db.UnmatchedPatreonPosts().ListPending(ctx)
	if len(list) != 1 || list[0].PublishedAt == nil || !list[0].PublishedAt.Equal(pub) {
		t.Fatalf("published_at round-trip failed: %+v", list[0])
	}
}
