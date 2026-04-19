package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/process"
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
	r.POST("/preview/revision/:id/edit", h.EditRevision)
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

// TestPreview_Edit_CreatesNewRevision verifies the load-bearing Task 24
// invariant: posting to /preview/revision/:id/edit MUST create a new
// revision row (source=manual_edit, status=pending_review, version bumped,
// edited_from_revision_id pointing at the source) and MUST leave the
// source revision's body/title/fingerprint untouched.
func TestPreview_Edit_CreatesNewRevision(t *testing.T) {
	r, db, _ := setupPreviewHandler(t)
	seedRevision(t, db, "c1", "pending_review")

	body := `{"title":"new title","body":"new body","author":"alice@example.com"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/preview/revision/c1/edit", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Admin-Key", "test-admin-key")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: %d body: %s", w.Code, w.Body.String())
	}

	// Confirm a NEW revision was created with source='manual_edit'
	all, _ := db.ContentRevisions().ListAll(context.Background(), "r")
	if len(all) != 2 {
		t.Fatalf("want 2 revisions after edit, got %d", len(all))
	}
	var newer *models.ContentRevision
	for _, rv := range all {
		if rv.ID != "c1" {
			newer = rv
		}
	}
	if newer == nil {
		t.Fatal("new revision not found")
	}
	if newer.Source != "manual_edit" {
		t.Fatalf("source: %s", newer.Source)
	}
	if newer.Version != 2 {
		t.Fatalf("version: %d want 2", newer.Version)
	}
	if newer.EditedFromRevisionID == nil || *newer.EditedFromRevisionID != "c1" {
		t.Fatalf("edited_from_revision_id: %v", newer.EditedFromRevisionID)
	}
	if newer.Status != "pending_review" {
		t.Fatalf("status: %s", newer.Status)
	}
	if newer.Title != "new title" || newer.Body != "new body" || newer.Author != "alice@example.com" {
		t.Fatalf("payload not persisted: %+v", newer)
	}

	// CRITICAL: original revision c1 must be UNTOUCHED
	orig, _ := db.ContentRevisions().GetByID(context.Background(), "c1")
	if orig.Title != "T" {
		t.Fatalf("c1 title mutated: %s", orig.Title)
	}
	if orig.Body != "B" {
		t.Fatalf("c1 body mutated: %s", orig.Body)
	}
	if orig.Fingerprint != "fp-c1" {
		t.Fatalf("c1 fingerprint mutated: %s", orig.Fingerprint)
	}
}

// TestPreview_Edit_ResponseBody pins the success response shape so clients
// can rely on {id, version} being present and correct.
func TestPreview_Edit_ResponseBody(t *testing.T) {
	r, db, _ := setupPreviewHandler(t)
	seedRevision(t, db, "c1", "pending_review")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/preview/revision/c1/edit",
		strings.NewReader(`{"title":"t","body":"b","author":"a"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Admin-Key", "test-admin-key")
	r.ServeHTTP(w, req)

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["id"] == nil || resp["id"] == "" {
		t.Fatalf("missing id: %+v", resp)
	}
	if v, _ := resp["version"].(float64); int(v) != 2 {
		t.Fatalf("version: %v want 2", resp["version"])
	}
}

// TestPreview_Edit_NoAuth_Unauthorized confirms the handler rejects
// requests without the X-Admin-Key header.
func TestPreview_Edit_NoAuth_Unauthorized(t *testing.T) {
	r, _, _ := setupPreviewHandler(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/preview/revision/c1/edit",
		strings.NewReader(`{"title":"t","body":"b","author":"a"}`))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

// TestPreview_Edit_MalformedJSON_BadRequest exercises the BindJSON error
// branch.
func TestPreview_Edit_MalformedJSON_BadRequest(t *testing.T) {
	r, db, _ := setupPreviewHandler(t)
	seedRevision(t, db, "c1", "pending_review")
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/preview/revision/c1/edit",
		strings.NewReader(`{not json`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Admin-Key", "test-admin-key")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

// TestPreview_Edit_EmptyFields_BadRequest confirms each required field is
// validated individually.
func TestPreview_Edit_EmptyFields_BadRequest(t *testing.T) {
	r, db, _ := setupPreviewHandler(t)
	seedRevision(t, db, "c1", "pending_review")
	cases := []string{
		`{"title":"","body":"b","author":"a"}`,
		`{"title":"t","body":"","author":"a"}`,
		`{"title":"t","body":"b","author":""}`,
	}
	for _, body := range cases {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/preview/revision/c1/edit", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Admin-Key", "test-admin-key")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("want 400 for %q, got %d", body, w.Code)
		}
	}
}

// TestPreview_Edit_NotFound covers the missing-id branch.
func TestPreview_Edit_NotFound(t *testing.T) {
	r, _, _ := setupPreviewHandler(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/preview/revision/nope/edit",
		strings.NewReader(`{"title":"t","body":"b","author":"a"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Admin-Key", "test-admin-key")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

// TestPreview_Edit_FingerprintIncludesNewBody asserts the new revision's
// fingerprint is recomputed from the new body (not copied from the source)
// and matches process.Fingerprint exactly — the handler must reuse the
// canonical fingerprint algorithm.
func TestPreview_Edit_FingerprintIncludesNewBody(t *testing.T) {
	r, db, _ := setupPreviewHandler(t)
	seedRevision(t, db, "c1", "pending_review")
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/preview/revision/c1/edit",
		strings.NewReader(`{"title":"t","body":"distinctive-body-text","author":"a"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Admin-Key", "test-admin-key")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status: %d", w.Code)
	}
	all, _ := db.ContentRevisions().ListAll(context.Background(), "r")
	for _, rv := range all {
		if rv.ID == "c1" {
			continue
		}
		if rv.Fingerprint == "fp-c1" {
			t.Fatalf("new revision reused old fingerprint")
		}
		expected := process.Fingerprint("distinctive-body-text", "")
		if rv.Fingerprint != expected {
			t.Fatalf("fingerprint: got %q want %q", rv.Fingerprint, expected)
		}
	}
}

// TestPreview_Edit_GetByIDErr_500 forces the initial GetByID to fail by
// closing the DB before the request. Exercises the 500 branch after auth
// and JSON validation pass but before MaxVersion/Create.
func TestPreview_Edit_GetByIDErr_500(t *testing.T) {
	r, db, _ := setupPreviewHandler(t)
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/preview/revision/c1/edit",
		strings.NewReader(`{"title":"t","body":"b","author":"a"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Admin-Key", "test-admin-key")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d body: %s", w.Code, w.Body.String())
	}
}

// TestPreview_Edit_CreateErr_500 exercises the 500 branch when Create
// fails. A BEFORE INSERT trigger aborts the INSERT while leaving SELECTs
// (GetByID, MaxVersion) intact, so the handler reaches Create and must
// surface the error as 500.
func TestPreview_Edit_CreateErr_500(t *testing.T) {
	r, db, _ := setupPreviewHandler(t)
	seedRevision(t, db, "c1", "pending_review")
	if _, err := db.DB().ExecContext(context.Background(),
		`CREATE TRIGGER block_rev_insert BEFORE INSERT ON content_revisions BEGIN SELECT RAISE(ABORT, 'blocked'); END`); err != nil {
		t.Fatalf("trigger: %v", err)
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/preview/revision/c1/edit",
		strings.NewReader(`{"title":"t","body":"b","author":"a"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Admin-Key", "test-admin-key")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d body: %s", w.Code, w.Body.String())
	}
}

// maxVersionErrStore wraps a real ContentRevisionStore so GetByID passes
// through, but MaxVersion always returns an error. The handler under test
// has three sequential DB touches — GetByID, MaxVersion, Create — and the
// MaxVersion 500 branch is the only one that can't be isolated via SQLite
// triggers (MaxVersion is a SELECT on the same table GetByID reads from,
// so a trigger either fails both or neither). A thin interface wrapper is
// the minimal-surface way to exercise exactly that branch.
type maxVersionErrStore struct {
	database.ContentRevisionStore
}

func (s *maxVersionErrStore) MaxVersion(ctx context.Context, repoID string) (int, error) {
	return 0, errors.New("max version boom")
}

// maxVersionErrDB wraps database.Database and swaps in maxVersionErrStore
// for ContentRevisions(); every other accessor is delegated to the
// embedded Database so the handler's untouched code paths still work.
type maxVersionErrDB struct {
	database.Database
	inner database.ContentRevisionStore
}

func (d *maxVersionErrDB) ContentRevisions() database.ContentRevisionStore {
	return &maxVersionErrStore{ContentRevisionStore: d.inner}
}

// TestPreview_Edit_MaxVersionErr_500 exercises the 500 branch for the
// MaxVersion call. We swap in a wrapper DB that delegates everything to
// the real SQLite DB except ContentRevisions(), which returns a store
// whose MaxVersion always errors — GetByID still succeeds so the handler
// reaches the MaxVersion call and must surface the error as 500.
func TestPreview_Edit_MaxVersionErr_500(t *testing.T) {
	gin.SetMode(gin.TestMode)
	real := testhelpers.OpenMigratedSQLite(t)
	seedRevision(t, real, "c1", "pending_review")
	wrappedDB := &maxVersionErrDB{Database: real, inner: real.ContentRevisions()}
	cfg := &config.Config{AdminKey: "test-admin-key"}
	h := handlers.NewPreviewHandler(wrappedDB, cfg)
	r := gin.New()
	r.POST("/preview/revision/:id/edit", h.EditRevision)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/preview/revision/c1/edit",
		strings.NewReader(`{"title":"t","body":"b","author":"a"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Admin-Key", "test-admin-key")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d body: %s", w.Code, w.Body.String())
	}
}
