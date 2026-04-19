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

func (h *PreviewHandler) Index(c *gin.Context) {
	ctx := c.Request.Context()

	var articles []ArticleView
	repos, err := h.db.Repositories().List(ctx, database.RepositoryFilter{})
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"Error": "Failed to load articles",
		})
		return
	}

	store := h.db.GeneratedContents()
	for _, repo := range repos {
		gc, err := store.GetLatestByRepo(ctx, repo.ID)
		if err != nil || gc == nil {
			continue
		}
		articles = append(articles, ArticleView{
			ID:           gc.ID,
			Title:        gc.Title,
			Status:       gc.Status,
			QualityScore: gc.QualityScore,
			CreatedAt:    gc.CreatedAt.Format("Jan 2, 2006"),
		})
	}

	c.HTML(http.StatusOK, "preview_index.html", gin.H{
		"Articles": articles,
	})
}

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
		preview.GET("/article/:id", h.ViewArticle)
		preview.GET("/edit/:id", h.EditArticle)
		preview.POST("/edit/:id", h.EditArticle)
		preview.POST("/toggle/:id/:action", h.ToggleArticle)
		preview.POST("/revision/:id/approve", h.ApproveRevision)
		preview.POST("/revision/:id/reject", h.RejectRevision)
		preview.POST("/revision/:id/edit", h.EditRevision)
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
	if c.GetHeader("X-Admin-Key") != h.config.AdminKey {
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
	if c.GetHeader("X-Admin-Key") != h.config.AdminKey {
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

type ArticleView struct {
	ID           string
	Title        string
	Body         string
	Status       string
	QualityScore float64
	CreatedAt    string
}
