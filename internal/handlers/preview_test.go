package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/milos85vasic/My-Patreon-Manager/internal/config"
	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/handlers"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/testhelpers"
)

// setupPreviewHandler constructs a Gin engine with only the approve/reject
// routes registered, plus a fully-migrated in-memory SQLite DB. The cfg's
// AdminKey is pinned to "test-admin-key" so tests can pass or omit that
// header to exercise the auth branches. The concrete *database.SQLiteDB is
// returned (not the Database interface) so tests can reach DB() for raw
// SQL seeding while still using the store methods for assertions.
func setupPreviewHandler(t *testing.T) (*gin.Engine, *database.SQLiteDB, *config.Config) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testhelpers.OpenMigratedSQLite(t)
	cfg := &config.Config{AdminKey: "test-admin-key"}
	h := handlers.NewPreviewHandler(db, cfg)
	r := gin.New()
	r.POST("/preview/revision/:id/approve", h.ApproveRevision)
	r.POST("/preview/revision/:id/reject", h.RejectRevision)
	return r, db, cfg
}

// seedRevision inserts a repository row (idempotent via UNIQUE-swallow) and
// a ContentRevision with the given id + status. Title/body/fingerprint are
// deterministic so tests can make equality assertions if needed.
func seedRevision(t *testing.T, db *database.SQLiteDB, id, status string) {
	t.Helper()
	_, err := db.DB().ExecContext(context.Background(),
		`INSERT INTO repositories (id, service, owner, name, url, https_url) VALUES ('r','github','o','n','u','h')`)
	if err != nil && !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatalf("seed repo: %v", err)
	}
	if err := db.ContentRevisions().Create(context.Background(), &models.ContentRevision{
		ID: id, RepositoryID: "r", Version: 1,
		Source: "generated", Status: status,
		Title: "T", Body: "B", Fingerprint: "fp-" + id,
		Author: "system", CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed revision %s: %v", id, err)
	}
}

func TestPreview_Approve_HappyPath(t *testing.T) {
	r, db, _ := setupPreviewHandler(t)
	seedRevision(t, db, "c1", models.RevisionStatusPendingReview)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/preview/revision/c1/approve", nil)
	req.Header.Set("X-Admin-Key", "test-admin-key")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: %d body: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v", err)
	}
	if resp["status"] != "approved" || resp["id"] != "c1" {
		t.Fatalf("resp: %+v", resp)
	}
	got, _ := db.ContentRevisions().GetByID(context.Background(), "c1")
	if got == nil || got.Status != models.RevisionStatusApproved {
		t.Fatalf("status: %+v", got)
	}
	// Body/title/fingerprint must remain immutable across the transition.
	if got.Title != "T" || got.Body != "B" || got.Fingerprint != "fp-c1" {
		t.Fatalf("immutable fields mutated: %+v", got)
	}
}

func TestPreview_Approve_NoAuth_Unauthorized(t *testing.T) {
	r, db, _ := setupPreviewHandler(t)
	seedRevision(t, db, "c1", models.RevisionStatusPendingReview)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/preview/revision/c1/approve", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
	// Status must not change when auth fails.
	got, _ := db.ContentRevisions().GetByID(context.Background(), "c1")
	if got.Status != models.RevisionStatusPendingReview {
		t.Fatalf("status changed despite auth failure: %s", got.Status)
	}
}

func TestPreview_Approve_WrongKey_Unauthorized(t *testing.T) {
	r, _, _ := setupPreviewHandler(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/preview/revision/c1/approve", nil)
	req.Header.Set("X-Admin-Key", "wrong")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

func TestPreview_Approve_NotFound(t *testing.T) {
	r, _, _ := setupPreviewHandler(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/preview/revision/missing/approve", nil)
	req.Header.Set("X-Admin-Key", "test-admin-key")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

func TestPreview_Approve_AlreadyApproved_BadRequest(t *testing.T) {
	r, db, _ := setupPreviewHandler(t)
	seedRevision(t, db, "c1", models.RevisionStatusApproved)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/preview/revision/c1/approve", nil)
	req.Header.Set("X-Admin-Key", "test-admin-key")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d body: %s", w.Code, w.Body.String())
	}
}

func TestPreview_Reject_HappyPath(t *testing.T) {
	r, db, _ := setupPreviewHandler(t)
	seedRevision(t, db, "c1", models.RevisionStatusPendingReview)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/preview/revision/c1/reject", nil)
	req.Header.Set("X-Admin-Key", "test-admin-key")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: %d body: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v", err)
	}
	if resp["status"] != "rejected" || resp["id"] != "c1" {
		t.Fatalf("resp: %+v", resp)
	}
	got, _ := db.ContentRevisions().GetByID(context.Background(), "c1")
	if got == nil || got.Status != models.RevisionStatusRejected {
		t.Fatalf("status: %+v", got)
	}
	if got.Title != "T" || got.Body != "B" || got.Fingerprint != "fp-c1" {
		t.Fatalf("immutable fields mutated: %+v", got)
	}
}

func TestPreview_Reject_FromApproved_BadRequest(t *testing.T) {
	r, db, _ := setupPreviewHandler(t)
	// approved -> rejected is illegal per the forward-only graph.
	seedRevision(t, db, "c1", models.RevisionStatusApproved)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/preview/revision/c1/reject", nil)
	req.Header.Set("X-Admin-Key", "test-admin-key")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d body: %s", w.Code, w.Body.String())
	}
}

func TestPreview_Reject_NoAuth_Unauthorized(t *testing.T) {
	r, _, _ := setupPreviewHandler(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/preview/revision/c1/reject", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

func TestPreview_Reject_WrongKey_Unauthorized(t *testing.T) {
	r, _, _ := setupPreviewHandler(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/preview/revision/c1/reject", nil)
	req.Header.Set("X-Admin-Key", "nope")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

func TestPreview_Reject_NotFound(t *testing.T) {
	r, _, _ := setupPreviewHandler(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/preview/revision/ghost/reject", nil)
	req.Header.Set("X-Admin-Key", "test-admin-key")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

// TestPreview_Approve_GetByIDErr_500 forces a store error on GetByID by
// closing the underlying database before the request. The handler must
// surface a 500 rather than a 404. Same strategy covers the GetByID
// failure branch for both transitionRevision callers.
func TestPreview_Approve_GetByIDErr_500(t *testing.T) {
	r, db, _ := setupPreviewHandler(t)
	// Close the DB so any subsequent query returns an error — not sql.ErrNoRows.
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/preview/revision/c1/approve", nil)
	req.Header.Set("X-Admin-Key", "test-admin-key")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d body: %s", w.Code, w.Body.String())
	}
}

// TestPreview_Reject_UpdateStatusErr_500 exercises the path where the
// handler's first GetByID succeeds but the subsequent UpdateStatus call
// fails with a generic (non-ErrIllegalStatusTransition) error — so the
// handler must return 500. We achieve this by installing a BEFORE UPDATE
// trigger on content_revisions that RAISEs an abort, while leaving the
// SELECT path intact. UpdateStatus internally re-reads first, confirms
// the transition is legal, then attempts the UPDATE — which the trigger
// aborts, producing a non-typed SQL error that the handler surfaces as 500.
func TestPreview_Reject_UpdateStatusErr_500(t *testing.T) {
	r, db, _ := setupPreviewHandler(t)
	seedRevision(t, db, "c1", models.RevisionStatusPendingReview)
	// Install a trigger that forces any UPDATE on content_revisions to fail.
	// SELECT still works, so the handler's GetByID and UpdateStatus's
	// internal legality check both succeed — only the UPDATE itself errors.
	if _, err := db.DB().ExecContext(context.Background(),
		`CREATE TRIGGER block_rev_update BEFORE UPDATE ON content_revisions BEGIN SELECT RAISE(ABORT, 'blocked'); END`); err != nil {
		t.Fatalf("trigger: %v", err)
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/preview/revision/c1/reject", nil)
	req.Header.Set("X-Admin-Key", "test-admin-key")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d body: %s", w.Code, w.Body.String())
	}
}
