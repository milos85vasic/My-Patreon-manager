package handlers

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/milos85vasic/My-Patreon-Manager/internal/config"
	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/process"
)

type PreviewHandler struct {
	db     database.Database
	config *config.Config
}

func NewPreviewHandler(db database.Database, cfg *config.Config) *PreviewHandler {
	return &PreviewHandler{db: db, config: cfg}
}

func (h *PreviewHandler) checkAuth(c *gin.Context) bool {
	key := c.GetHeader("X-Admin-Key")
	if key == "" {
		key = c.GetHeader("X-Reviewer-Key")
	}
	if key == "" {
		return false
	}
	adminMatch := key == h.config.AdminKey
	reviewerMatch := h.config.ReviewerKey != "" && key == h.config.ReviewerKey
	return adminMatch || reviewerMatch
}

// repoDashboardRow is the per-row data shape rendered on the preview
// dashboard (GET /preview). It captures just the handful of fields the
// operator needs to triage a repo: current process_state, counts of
// revisions pending review and approved (i.e. awaiting publish), and the
// two revision pointer IDs. HasDrift is a convenience flag derived from
// ProcessState so the template doesn't need to string-compare.
type repoDashboardRow struct {
	ID                  string
	Name                string
	Service             string
	ProcessState        string
	PendingReviewCount  int
	ApprovedCount       int
	HasDrift            bool
	CurrentRevisionID   string
	PublishedRevisionID string
}

// Index renders the repository dashboard at GET /preview. Rows are
// surfaced in fair-queue order (least-recently-processed first) via
// RepositoryStore.ListForProcessQueue, which matches the order the
// process command would pick them. For each repo we aggregate:
//
//   - PendingReviewCount: revisions awaiting operator approval
//   - ApprovedCount:      revisions approved and awaiting publish
//   - HasDrift:           repo is in patreon_drift_detected state
//
// A template is required (see cmd/server/main.go's ParseGlob) — if the
// template registry is empty, Gin will abort the response with 500,
// which is acceptable in tests that don't register templates.
func (h *PreviewHandler) Index(c *gin.Context) {
	ctx := c.Request.Context()
	repos, err := h.db.Repositories().ListForProcessQueue(ctx)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"Error": "Failed to load repos",
		})
		return
	}
	rows := make([]repoDashboardRow, 0, len(repos))
	for _, r := range repos {
		pending, _ := h.db.ContentRevisions().ListByRepoStatus(ctx, r.ID, models.RevisionStatusPendingReview)
		approved, _ := h.db.ContentRevisions().ListByRepoStatus(ctx, r.ID, models.RevisionStatusApproved)
		row := repoDashboardRow{
			ID:                 r.ID,
			Name:               r.Owner + "/" + r.Name,
			Service:            r.Service,
			ProcessState:       r.ProcessState,
			PendingReviewCount: len(pending),
			ApprovedCount:      len(approved),
			HasDrift:           r.ProcessState == "patreon_drift_detected",
		}
		if r.CurrentRevisionID != nil {
			row.CurrentRevisionID = *r.CurrentRevisionID
		}
		if r.PublishedRevisionID != nil {
			row.PublishedRevisionID = *r.PublishedRevisionID
		}
		rows = append(rows, row)
	}
	c.HTML(http.StatusOK, "preview_index.html", gin.H{"Repos": rows})
}

// RepoHistory renders the per-repo revision timeline at
// GET /preview/repo/:repo_id. Revisions are loaded via
// ContentRevisionStore.ListAll which returns rows in version DESC order
// — the newest entry appears at the top of the template. A nil repo
// (unknown :repo_id) yields a bare 404; store errors surface as 500 via
// the error.html template.
//
// Registered at /preview/repo/:repo_id (not /preview/:repo_id) to avoid
// a Gin routing collision with the existing /preview/article/:id and
// /preview/edit/:id routes: Gin rejects ambiguous dynamic prefixes at
// registration time.
func (h *PreviewHandler) RepoHistory(c *gin.Context) {
	ctx := c.Request.Context()
	repoID := c.Param("repo_id")
	repo, err := h.db.Repositories().GetByID(ctx, repoID)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"Error": err.Error()})
		return
	}
	if repo == nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	revs, err := h.db.ContentRevisions().ListAll(ctx, repoID)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"Error": err.Error()})
		return
	}
	c.HTML(http.StatusOK, "preview_repo.html", gin.H{
		"Repo":      repo,
		"Revisions": revs,
	})
}

// ViewArticle and EditArticle below are legacy handlers that operate on
// the generated_contents table. They remain registered so existing
// deep links keep working during the process-command rollout, but will
// retire in Task 33 once the dashboard + per-repo history replaces the
// "article" surface entirely. New preview UI work should target the
// content_revisions-backed handlers (Index, RepoHistory, ApproveRevision,
// etc.) instead.

func (h *PreviewHandler) ViewArticle(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")

	gc, err := h.db.GeneratedContents().GetByID(ctx, id)
	if err != nil || gc == nil {
		c.HTML(http.StatusNotFound, "error.html", gin.H{
			"Error": "Article not found",
		})
		return
	}

	c.HTML(http.StatusOK, "preview_article.html", gin.H{
		"Article": ArticleView{
			ID:           gc.ID,
			Title:        gc.Title,
			Body:         gc.Body,
			Status:       gc.Status,
			QualityScore: gc.QualityScore,
			CreatedAt:    gc.CreatedAt.Format("Jan 2, 2006"),
		},
	})
}

func (h *PreviewHandler) EditArticle(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")

	if c.Request.Method == http.MethodPost {
		title := c.PostForm("title")
		body := c.PostForm("body")
		status := c.PostForm("status")

		gc, err := h.db.GeneratedContents().GetByID(ctx, id)
		if err != nil || gc == nil {
			c.HTML(http.StatusNotFound, "error.html", gin.H{
				"Error": "Article not found",
			})
			return
		}

		if title != "" {
			gc.Title = title
		}
		if body != "" {
			gc.Body = body
		}
		if status != "" {
			gc.Status = status
		}

		if err := h.db.GeneratedContents().Update(ctx, gc); err != nil {
			c.HTML(http.StatusInternalServerError, "error.html", gin.H{
				"Error": "Failed to save article",
			})
			return
		}

		c.Redirect(http.StatusFound, "/preview/article/"+id)
		return
	}

	gc, err := h.db.GeneratedContents().GetByID(ctx, id)
	if err != nil || gc == nil {
		c.HTML(http.StatusNotFound, "error.html", gin.H{
			"Error": "Article not found",
		})
		return
	}

	c.HTML(http.StatusOK, "preview_edit.html", gin.H{
		"Article": ArticleView{
			ID:           gc.ID,
			Title:        gc.Title,
			Body:         gc.Body,
			Status:       gc.Status,
			QualityScore: gc.QualityScore,
		},
		"MDEVersion": "4.6.0",
	})
}

func (h *PreviewHandler) ToggleArticle(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")
	action := c.Param("action")

	gc, err := h.db.GeneratedContents().GetByID(ctx, id)
	if err != nil || gc == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Article not found"})
		return
	}

	switch action {
	case "enable":
		gc.Status = "published"
	case "disable":
		gc.Status = "draft"
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid action"})
		return
	}

	if err := h.db.GeneratedContents().Update(ctx, gc); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update article"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": gc.Status})
}

func (h *PreviewHandler) RegisterRoutes(r *gin.Engine) {
	preview := r.Group("/preview")
	{
		preview.GET("", h.Index)
		preview.GET("/repo/:repo_id", h.RepoHistory)
		preview.GET("/article/:id", h.ViewArticle)
		preview.GET("/edit/:id", h.EditArticle)
		preview.POST("/edit/:id", h.EditArticle)
		preview.POST("/toggle/:id/:action", h.ToggleArticle)
		preview.POST("/revision/:id/approve", h.ApproveRevision)
		preview.POST("/revision/:id/reject", h.RejectRevision)
		preview.POST("/revision/:id/edit", h.EditRevision)
		preview.POST("/:repo_id/resolve-drift", h.ResolveDrift)
	}
}

// editRevisionRequest is the JSON body for POST /preview/revision/:id/edit.
// All three fields are required — empty strings are rejected as 400.
type editRevisionRequest struct {
	Title  string `json:"title"`
	Body   string `json:"body"`
	Author string `json:"author"`
}

// EditRevision creates a NEW content_revisions row that supersedes the
// target revision without mutating it. This is a load-bearing safety
// invariant: Task 24 of the process-command plan requires that the
// original revision's body/title/fingerprint remain literally unchanged
// after an edit — the edit materializes as a fresh pending_review row
// with edited_from_revision_id pointing back at the source.
//
// Returns:
//   - 200 with {"id":<new-id>, "version":<new-version>} on success,
//   - 400 on malformed JSON or any empty required field,
//   - 401 when X-Admin-Key is missing or wrong,
//   - 404 when :id does not exist,
//   - 500 on any other store error.
func (h *PreviewHandler) EditRevision(c *gin.Context) {
	if !h.checkAuth(c) {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	var req editRevisionRequest
	if err := c.BindJSON(&req); err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	if req.Title == "" || req.Body == "" || req.Author == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title, body, and author are required"})
		return
	}
	ctx := c.Request.Context()
	id := c.Param("id")
	cur, err := h.db.ContentRevisions().GetByID(ctx, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if cur == nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	maxV, err := h.db.ContentRevisions().MaxVersion(ctx, cur.RepositoryID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	newID := uuid.NewString()
	editedFromID := cur.ID
	next := &models.ContentRevision{
		ID:                   newID,
		RepositoryID:         cur.RepositoryID,
		Version:              maxV + 1,
		Source:               "manual_edit",
		Status:               models.RevisionStatusPendingReview,
		Title:                req.Title,
		Body:                 req.Body,
		Fingerprint:          process.Fingerprint(req.Body, ""),
		EditedFromRevisionID: &editedFromID,
		Author:               req.Author,
		CreatedAt:            time.Now().UTC(),
	}
	if err := h.db.ContentRevisions().Create(ctx, next); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": newID, "version": next.Version})
}

// ApproveRevision transitions a ContentRevision from pending_review to
// approved. Requires X-Admin-Key to match cfg.AdminKey. Returns:
//   - 200 with {"status":"approved","id":<id>} on success,
//   - 400 if the current status forbids the transition,
//   - 401 if the admin key is missing or wrong,
//   - 404 if the revision does not exist,
//   - 500 on any other store error.
//
// The handler is a thin wrapper over ContentRevisionStore.UpdateStatus,
// which enforces the forward-only status graph. Body/title/fingerprint
// are never touched.
func (h *PreviewHandler) ApproveRevision(c *gin.Context) {
	h.transitionRevision(c, models.RevisionStatusApproved, "approved")
}

// RejectRevision transitions a ContentRevision from pending_review to
// rejected. Same semantics as ApproveRevision; see that doc for the
// response shape and error codes.
func (h *PreviewHandler) RejectRevision(c *gin.Context) {
	h.transitionRevision(c, models.RevisionStatusRejected, "rejected")
}

// transitionRevision is the shared implementation of ApproveRevision and
// RejectRevision. The responseStatus parameter is the string returned to
// the client in the "status" JSON field on success; it mirrors the store
// status but is kept as a separate argument so the caller controls the
// wire shape explicitly.
func (h *PreviewHandler) transitionRevision(c *gin.Context, newStatus, responseStatus string) {
	if !h.checkAuth(c) {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	id := c.Param("id")
	ctx := c.Request.Context()
	cur, err := h.db.ContentRevisions().GetByID(ctx, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if cur == nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	if err := h.db.ContentRevisions().UpdateStatus(ctx, id, newStatus); err != nil {
		if errors.Is(err, database.ErrIllegalStatusTransition) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": responseStatus, "id": id})
}

// resolveDriftRequest is the JSON body for POST /preview/:repo_id/resolve-drift.
// Resolution must be exactly "keep_ours" or "keep_theirs"; any other value is
// rejected as a 400.
type resolveDriftRequest struct {
	Resolution string `json:"resolution"`
}

// ResolveDrift resolves a patreon_drift_detected state on a repository.
//
// Two resolutions are supported via the JSON body's "resolution" field:
//
//   - keep_ours:    operator re-publishes our last-approved revision,
//                   overriding the external Patreon edit. The existing
//                   patreon_import revision is kept in history as an audit
//                   trail; no revision statuses change. Repo state flips
//                   back to idle so the publisher can act again.
//
//   - keep_theirs:  operator accepts the external Patreon edit as canonical.
//                   The most recent patreon_import revision (version DESC) is
//                   promoted to current_revision_id AND published_revision_id.
//                   Older approved generated revisions for this repo are
//                   superseded (approved -> superseded is legal). Any
//                   currently pending_review drafts are rejected
//                   (pending_review -> rejected is legal). Repo state flips
//                   back to idle.
//
// Returns:
//   - 200 with {"resolution":<r>, "repo_id":<id>} on success,
//   - 400 on malformed JSON, unknown resolution, or keep_theirs with no
//         patreon_import revision on file,
//   - 401 on missing or wrong X-Admin-Key,
//   - 404 when :repo_id doesn't exist,
//   - 409 when the repo isn't in patreon_drift_detected state (you cannot
//         "resolve" a non-drifted repo),
//   - 500 on any store error.
func (h *PreviewHandler) ResolveDrift(c *gin.Context) {
	if !h.checkAuth(c) {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	var req resolveDriftRequest
	if err := c.BindJSON(&req); err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	if req.Resolution != "keep_ours" && req.Resolution != "keep_theirs" {
		c.JSON(http.StatusBadRequest, gin.H{"error": `resolution must be "keep_ours" or "keep_theirs"`})
		return
	}

	ctx := c.Request.Context()
	repoID := c.Param("repo_id")

	repo, err := h.db.Repositories().GetByID(ctx, repoID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if repo == nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	if repo.ProcessState != "patreon_drift_detected" {
		c.JSON(http.StatusConflict, gin.H{
			"error":         "repo is not in patreon_drift_detected state",
			"process_state": repo.ProcessState,
		})
		return
	}

	if req.Resolution == "keep_ours" {
		if err := h.db.Repositories().SetProcessState(ctx, repoID, "idle"); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"resolution": "keep_ours", "repo_id": repoID})
		return
	}

	// keep_theirs
	all, err := h.db.ContentRevisions().ListAll(ctx, repoID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var mostRecentImport *models.ContentRevision
	for _, rv := range all { // ListAll returns version DESC.
		if rv.Source == "patreon_import" {
			mostRecentImport = rv
			break
		}
	}
	if mostRecentImport == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no patreon_import revision found for this repo; cannot keep_theirs"})
		return
	}

	// Supersede older approved generated revisions; reject any pending_review
	// drafts. Both are legal forward-only transitions per the status graph.
	for _, rv := range all {
		if rv.ID == mostRecentImport.ID {
			continue
		}
		switch rv.Status {
		case models.RevisionStatusApproved:
			if rv.Version < mostRecentImport.Version {
				if err := h.db.ContentRevisions().UpdateStatus(ctx, rv.ID, models.RevisionStatusSuperseded); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
			}
		case models.RevisionStatusPendingReview:
			if err := h.db.ContentRevisions().UpdateStatus(ctx, rv.ID, models.RevisionStatusRejected); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
	}

	if err := h.db.Repositories().SetRevisionPointers(ctx, repoID, mostRecentImport.ID, mostRecentImport.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.db.Repositories().SetProcessState(ctx, repoID, "idle"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"resolution": "keep_theirs", "repo_id": repoID})
}

type ArticleView struct {
	ID           string
	Title        string
	Body         string
	Status       string
	QualityScore float64
	CreatedAt    string
}
