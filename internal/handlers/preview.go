package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/milos85vasic/My-Patreon-Manager/internal/config"
	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
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
	}
}

type ArticleView struct {
	ID           string
	Title        string
	Body         string
	Status       string
	QualityScore float64
	CreatedAt    string
}
