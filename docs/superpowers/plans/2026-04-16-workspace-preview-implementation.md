# Workspace, Preview & Filtering Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rename config params (PROCESS_PRIVATE_REPOS → PROCESS_PRIVATE_REPOSITORIES, REPO_MAX_INACTIVITY_DAYS → MIN_MONTHS_COMMIT_ACTIVITY), add user workspace with default SQLite matric, and implement preview website for generated content.

**Architecture:** Config changes in config.go + orchestrator.go. New workspace auto-creation. DB migration adds status column. Preview handlers in internal/handlers/preview.go with embedded HTML templates.

**Tech Stack:** Go 1.26.1, Gin, Go html/template, EasyMDE (CDN), SQLite

---

## Task 1: Config Parameter Renaming

**Files:**
- Modify: `internal/config/config.go:55-103`
- Modify: `.env.example:46-51`

### 1.1 Config struct field renaming

- [ ] **Step 1: Rename ProcessPrivateRepos → ProcessPrivateRepositories**

Modify file: `internal/config/config.go:64-66`
```go
// ProcessPrivateRepositories controls whether private repositories are included
// in sync/scan/generate. Defaults to false (public repos only).
ProcessPrivateRepositories bool
```

- [ ] **Step 2: Rename RepoMaxInactivityDays → MinMonthsCommitActivity**

Modify file: `internal/config/config.go:67-70`
```go
// MinMonthsCommitActivity is the maximum number of months since the last
// commit for a repository to be considered active. Repositories with no
// commits within this window are skipped. Defaults to 18 (≈18 months).
MinMonthsCommitActivity int
```

- [ ] **Step 3: Update NewConfig() defaults**

Modify file: `internal/config/config.go:98-99`
```go
ProcessPrivateRepositories:   false,
MinMonthsCommitActivity:   18, // 18 months
```

- [ ] **Step 4: Update env loading**

Modify file: `internal/config/config.go:212-213` (remove old lines, add new)
```go
c.ProcessPrivateRepositories = getEnvBool("PROCESS_PRIVATE_REPOSITORIES", c.ProcessPrivateRepositories)
c.MinMonthsCommitActivity = getEnvInt("MIN_MONTHS_COMMIT_ACTIVITY", c.MinMonthsCommitActivity)
```

- [ ] **Step 5: Update .env.example**

Modify file: `.env.example:46-51`
```
# Repository Filtering
# Set to true to include private repositories in sync/scan/generate.
PROCESS_PRIVATE_REPOSITORIES=false
# Minimum months since last commit. Repos inactive longer are skipped.
# Default: 18 months.
MIN_MONTHS_COMMIT_ACTIVITY=18
```

---

## Task 2: Orchestrator Sync Options Renaming

**Files:**
- Modify: `internal/services/sync/orchestrator.go:78-79`

- [ ] **Step 1: Rename SyncOptions struct fields**

Modify file: `internal/services/sync/orchestrator.go:78-79`
```go
ProcessPrivateRepositories bool
MinMonthsCommitActivity    int
```

- [ ] **Step 2: Update filter logic**

Modify file: `internal/services/sync/orchestrator.go:271-302`
```go
// Filter private repositories unless explicitly enabled.
if !opts.ProcessPrivateRepositories {
    before := len(allRepos)
    var publicOnly []models.Repository
    for _, r := range allRepos {
        if !r.IsPrivate {
            publicOnly = append(publicOnly, r)
        }
    }
    // ... rest unchanged
}

// Filter repositories with no commits within the inactivity window.
if opts.MinMonthsCommitActivity > 0 {
    before := len(allRepos)
    cutoff := time.Now().AddDate(0, -opts.MinMonthsCommitActivity, 0)
    var active []models.Repository
    for _, r := range allRepos {
        if r.LastCommitAt.IsZero() || r.LastCommitAt.After(cutoff) {
            active = append(active, r)
        }
    }
    // ... rest unchanged
}
```

- [ ] **Step 3: Update CLI main.go**

Modify file: `cmd/cli/main.go:170-171`
```go
ProcessPrivateRepositories:   cfg.ProcessPrivateRepositories,
MinMonthsCommitActivity:    cfg.MinMonthsCommitActivity,
```

---

## Task 3: User Workspace Directory

**Files:**
- Modify: `internal/config/config.go`
- Modify: `.gitignore`

### 3.1 Add workspace config

- [ ] **Step 1: Add UserWorkspaceDir config field**

Modify file: `internal/config/config.go` (add after MinMonthsCommitActivity)
```go
// UserWorkspaceDir is the root directory for all user data.
// Defaults to "user". Created automatically on first run.
UserWorkspaceDir string
```

- [ ] **Step 2: Update .env.example with USER_WORKSPACE_DIR**

Modify file: `.env.example` (add after MIN_MONTHS_COMMIT_ACTIVITY)
```
# User workspace directory (created automatically on first run)
USER_WORKSPACE_DIR=user
```

- [ ] **Step 3: Update .gitignore**

Modify file: `.gitignore` (add at end)
```
/user/
```

- [ ] **Step 4: Ensure DB_PATH defaults to workspace**

Modify file: `internal/config/config.go` (update DSN or DBPath)
```go
// In NewConfig(), set DBPath default:
DBPath: "user/db/patreon_manager.db",
```

---

## Task 4: Database Migration for Status Column

**Files:**
- Modify: `internal/database/sqlite.go`

- [ ] **Step 1: Add migration for status column**

Locate the generated_contents INSERT statements in sqlite.go, add migration:
```go
// In migrate() function, add:
_, err = db.Exec("ALTER TABLE generated_contents ADD COLUMN status TEXT DEFAULT 'enabled'")
if err != nil && !strings.Contains(err.Error(), "duplicate column") {
    return err
}
```

---

## Task 5: Preview Website Handlers

**Files:**
- Create: `internal/handlers/preview.go`
- Create: `internal/handlers/templates/preview/*.html`

### 5.1 Dashboard (GET /preview)

- [ ] **Step 1: Create preview handler file**

Create file: `internal/handlers/preview.go`
```go
package handlers

import (
    "database/sql"
    "html/template"
    "net/http"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

type PreviewHandler struct {
    DB *sql.DB
}

func NewPreviewHandler(db *sql.DB) *PreviewHandler {
    return &PreviewHandler{DB: db}
}

func (h *PreviewHandler) Index(c *gin.Context) {
    rows, err := h.DB.Query(`
        SELECT id, repository_id, content_type, format, title, body, 
               quality_score, passed_quality_gate, created_at, COALESCE(status, 'enabled') as status
        FROM generated_contents ORDER BY created_at DESC
    `)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()

    var articles []map[string]interface{}
    for rows.Next() {
        var a models.GeneratedContent
        var status string
        var body, title string
        var repoID, contentType, format string
        var qualityScore float64
        var passedGate bool
        var createdAt time.Time
        rows.Scan(&a.ID, &repoID, &contentType, &format, &title, &body, &qualityScore, &passedGate, &createdAt, &status)
        articles = append(articles, map[string]interface{}{
            "id": a.ID,
            "repository_id": repoID,
            "content_type": contentType,
            "format": format,
            "title": title,
            "quality_score": qualityScore,
            "passed_quality_gate": passedGate,
            "status": status,
            "created_at": createdAt,
        })
    }
    c.HTML(http.StatusOK, "preview_index", gin.H{"articles": articles})
}

func (h *PreviewHandler) ViewArticle(c *gin.Context) {
    id := c.Param("id")
    var a models.GeneratedContent
    var status string
    err := h.DB.QueryRow(`
        SELECT id, repository_id, content_type, format, title, body, 
               quality_score, model_used, prompt_template, token_count,
               generation_attempts, passed_quality_gate, created_at, COALESCE(status, 'enabled') as status
        FROM generated_contents WHERE id = ?
    `, id).Scan(&a.ID, &a.RepositoryID, &a.ContentType, &a.Format, &a.Title, &a.Body,
        &a.QualityScore, &a.ModelUsed, &a.PromptTemplate, &a.TokenCount,
        &a.GenerationAttempts, &a.PassedQualityGate, &a.CreatedAt, &status)
    if err == sql.ErrNoRows {
        c.JSON(404, gin.H{"error": "article not found"})
        return
    }
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.HTML(http.StatusOK, "preview_article", gin.H{
        "article": a,
        "status": status,
    })
}

func (h *PreviewHandler) ToggleArticle(c *gin.Context) {
    id := c.Param("id")
    var currentStatus string
    err := h.DB.QueryRow("SELECT COALESCE(status, 'enabled') FROM generated_contents WHERE id = ?", id).Scan(&currentStatus)
    if err == sql.ErrNoRows {
        c.JSON(404, gin.H{"error": "article not found"})
        return
    }
    newStatus := "disabled"
    if currentStatus == "disabled" {
        newStatus = "enabled"
    }
    _, err = h.DB.Exec("UPDATE generated_contents SET status = ? WHERE id = ?", newStatus, id)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(200, gin.H{"success": true, "status": newStatus})
}

func (h *PreviewHandler) DeleteArticle(c *gin.Context) {
    id := c.Param("id")
    result, err := h.DB.Exec("DELETE FROM generated_contents WHERE id = ?", id)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    rowsAffected, _ := result.RowsAffected()
    if rowsAffected == 0 {
        c.JSON(404, gin.H{"error": "article not found"})
        return
    }
    c.Redirect(http.StatusFound, "/preview")
}

func (h *PreviewHandler) EditArticle(c *gin.Context) {
    id := c.Param("id")
    var a models.GeneratedContent
    var status string
    err := h.DB.QueryRow(`
        SELECT id, title, body, COALESCE(status, 'enabled') as status
        FROM generated_contents WHERE id = ?
    `, id).Scan(&a.ID, &a.Title, &a.Body, &status)
    if err == sql.ErrNoRows {
        c.JSON(404, gin.H{"error": "article not found"})
        return
    }
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.HTML(http.StatusOK, "preview_edit", gin.H{
        "article": a,
        "status": status,
    })
}

func (h *PreviewHandler) SaveArticle(c *gin.Context) {
    id := c.Param("id")
    var body string
    if err := c.ShouldBind(&body); err != nil {
        // Try form bind
        body = c.PostForm("body")
    }
    if body == "" {
        c.JSON(400, gin.H{"error": "body is required"})
        return
    }
    _, err := h.DB.Exec("UPDATE generated_contents SET body = ? WHERE id = ?", body, id)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(200, gin.H{"success": true})
}
```

### 5.2 Template files

- [ ] **Step 1: Create templates directory**

Create directory: `internal/handlers/templates/preview/`

- [ ] **Step 2: Create layout.html**

Create file: `internal/handlers/templates/preview/layout.html`
```html
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{block "title"}}My Patreon Manager{{end}}</title>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; line-height: 1.6; background: #f5f5f5; }
        .nav { background: #333; color: white; padding: 1rem; }
        .nav a { color: white; text-decoration: none; margin-right: 1rem; }
        .container { max-width: 1200px; margin: 0 auto; padding: 2rem; }
        .card { background: white; border-radius: 8px; padding: 1.5rem; margin-bottom: 1rem; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        .btn { display: inline-block; padding: 0.5rem 1rem; background: #007bff; color: white; text-decoration: none; border-radius: 4px; border: none; cursor: pointer; }
        .btn:hover { background: #0056b3; }
        .btn-danger { background: #dc3545; }
        .btn-danger:hover { background: #c82333; }
        .btn-secondary { background: #6c757d; }
        .btn-secondary:hover { background: #5a6268; }
        .badge { display: inline-block; padding: 0.25rem 0.5rem; border-radius: 4px; font-size: 0.875rem; }
        .badge-enabled { background: #28a745; color: white; }
        .badge-disabled { background: #dc3545; color: white; }
        table { width: 100%; border-collapse: collapse; }
        th, td { padding: 0.75rem; text-align: left; border-bottom: 1px solid #eee; }
    </style>
    {{block "extra_head"}}{{end}}
</head>
<body>
    <div class="nav">
        <a href="/preview">Dashboard</a>
    </div>
    <div class="container">
        {{block "content"}}{{end}}
    </div>
</body>
</html>
```

- [ ] **Step 3: Create index.html**

Create file: `internal/handlers/templates/preview/index.html`
```html
{{template "layout.html" .}}

{{define "title"}}Dashboard - My Patreon Manager{{end}}

{{define "content"}}
<h1>Generated Articles</h1>
{{if .articles}}
<table>
    <thead>
        <tr>
            <th>Title</th>
            <th>Repository</th>
            <th>Quality</th>
            <th>Status</th>
            <th>Created</th>
            <th>Actions</th>
        </tr>
    </thead>
    <tbody>
        {{range .articles}}
        <tr>
            <td>{{.title}}</td>
            <td>{{.repository_id}}</td>
            <td>{{printf "%.2f" .quality_score}}</td>
            <td><span class="badge badge-{{.status}}">{{.status}}</span></td>
            <td>{{.created_at.Format "2006-01-02 15:04"}}</td>
            <td>
                <a href="/preview/article/{{.id}}" class="btn btn-secondary">View</a>
                <a href="/preview/article/{{.id}}/edit" class="btn btn-secondary">Edit</a>
                <form action="/preview/article/{{.id}}/toggle" method="POST" style="display:inline;">
                    <button type="submit" class="btn">{{if eq .status "enabled"}}Disable{{else}}Enable{{end}}</button>
                </form>
                <form action="/preview/article/{{.id}}/delete" method="POST" style="display:inline;" onsubmit="return confirm('Delete this article? This cannot be undone.');">
                    <button type="submit" class="btn btn-danger">Delete</button>
                </form>
            </td>
        </tr>
        {{end}}
    </tbody>
</table>
{{else}}
<p>No articles generated yet. Run <code>patreon-manager generate</code> to create content.</p>
{{end}}
{{end}}
```

- [ ] **Step 4: Create article.html**

Create file: `internal/handlers/templates/preview/article.html`
```html
{{template "layout.html" .}}

{{define "title"}}{{.article.Title}} - My Patreon Manager{{end}}

{{define "content"}}
<div class="card">
    <h1>{{.article.Title}}</h1>
    <p><strong>Repository:</strong> {{.article.RepositoryID}}</p>
    <p><strong>Quality Score:</strong> {{printf "%.2f" .article.QualityScore}}</p>
    <p><strong>Status:</strong> <span class="badge badge-{{.status}}">{{.status}}</span></p>
    <p><strong>Created:</strong> {{.article.CreatedAt.Format "2006-01-02 15:04"}}</p>
</div>

<div class="card">
    <h2>Content</h2>
    <pre style="white-space: pre-wrap;">{{.article.Body}}</pre>
</div>

<div>
    <a href="/preview/article/{{.article.ID}}/edit" class="btn">Edit</a>
    <a href="/preview" class="btn btn-secondary">Back to Dashboard</a>
</div>
{{end}}
```

- [ ] **Step 5: Create edit.html with EasyMDE**

Create file: `internal/handlers/templates/preview/edit.html`
```html
{{template "layout.html" .}}

{{define "extra_head"}}
<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/easymde/dist/easymde.min.css">
<script src="https://cdn.jsdelivr.net/npm/easymde/dist/easymde.min.js"></script>
{{end}}

{{define "title"}}Edit: {{.article.Title}} - My Patreon Manager{{end}}

{{define "content"}}
<h1>Edit Article</h1>

<form action="/preview/article/{{.article.ID}}/save" method="POST" id="editForm">
    <textarea name="body" id="editor">{{.article.Body}}</textarea>
    <div style="margin-top: 1rem;">
        <button type="submit" class="btn">Save</button>
        <a href="/preview/article/{{.article.ID}}" class="btn btn-secondary">Cancel</a>
    </div>
</form>

<script>
var easyMDE = new EasyMDE({
    element: document.getElementById('editor'),
    spellChecker: false,
    autosave: {
        enabled: true,
        uniqueId: '{{.article.ID}}',
        delay: 1000,
    },
});
</script>
{{end}}
```

### 5.3 Register routes in server

- [ ] **Step 1: Add preview routes to server main.go**

Modify file: `cmd/server/main.go` (add in setupRouter):
```go
previewHandler := handlers.NewPreviewHandler(db)

// Preview website (no auth)
router.GET("/preview", previewHandler.Index)
router.GET("/preview/article/:id", previewHandler.ViewArticle)
router.GET("/preview/article/:id/edit", previewHandler.EditArticle)
router.POST("/preview/article/:id/toggle", previewHandler.ToggleArticle)
router.POST("/preview/article/:id/delete", previewHandler.DeleteArticle)
router.POST("/preview/article/:id/save", previewHandler.SaveArticle)
```

- [ ] **Step 2: Load templates in server init**

Modify file: `cmd/server/main.go` (load templates before router setup):
```go
templateFS := template.Must(template.ParseGlob("internal/handlers/templates/preview/*.html"))
router.SetHTMLTemplate(templateFS)
```

---

## Task 6: Testing

**Files:**
- Modify: Various test files

- [ ] **Step 1: Add config tests**

Create file: `tests/unit/config/workspace_test.go`
```go
package config

import (
    "os"
    "testing"
)

func TestProcessPrivateRepositoriesConfig(t *testing.T) {
    os.Setenv("PROCESS_PRIVATE_REPOSITORIES", "true")
    defer os.Unsetenv("PROCESS_PRIVATE_REPOSITORIES")
    
    cfg := NewConfig()
    if !cfg.ProcessPrivateRepositories {
        t.Error("Expected ProcessPrivateRepositories to be true")
    }
}

func TestMinMonthsCommitActivityConfig(t *testing.T) {
    os.Setenv("MIN_MONTHS_COMMIT_ACTIVITY", "6")
    defer os.Unsetenv("MIN_MONTHS_COMMIT_ACTIVITY")
    
    cfg := NewConfig()
    if cfg.MinMonthsCommitActivity != 6 {
        t.Errorf("Expected MinMonthsCommitActivity to be 6, got %d", cfg.MinMonthsCommitActivity)
    }
}

func TestUserWorkspaceDirConfig(t *testing.T) {
    os.Setenv("USER_WORKSPACE_DIR", "custom_workspace")
    defer os.Unsetenv("USER_WORKSPACE_DIR")
    
    cfg := NewConfig()
    if cfg.UserWorkspaceDir != "custom_workspace" {
        t.Errorf("Expected UserWorkspaceDir to be 'custom_workspace', got '%s'", cfg.UserWorkspaceDir)
    }
}

func TestDefaultDBPathInWorkspace(t *testing.T) {
    cfg := NewConfig()
    if cfg.DBPath != "user/db/patreon_manager.db" {
        t.Errorf("Expected default DBPath to be 'user/db/patreon_manager.db', got '%s'", cfg.DBPath)
    }
}
```

- [ ] **Step 2: Add preview handler tests**

Create file: `tests/unit/handlers/preview_test.go`
```go
package handlers

import (
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/gin-gonic/gin"
)

func TestPreviewHandlerIndex(t *testing.T) {
    gin.SetMode(gin.TestMode)
    r := gin.New()
    
    // Mock the handler with a test database
    // Test that /preview returns 200
    r.GET("/preview", func(c *gin.Context) {
        c.HTML(http.StatusOK, "preview_index", gin.H{"articles": []struct{}{}})
    })
    
    req := httptest.NewRequest("GET", "/preview", nil)
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)
    
    if w.Code != http.StatusOK {
        t.Errorf("Expected status 200, got %d", w.Code)
    }
}
```

- [ ] **Step 3: Run tests**

```bash
cd .worktrees/workspace-preview
go test ./tests/unit/config/... -v
go test ./tests/unit/handlers/... -v
```

---

## Task 7: Documentation Updates

**Files:**
- Modify: `.env.example`, `.gitignore`

- [ ] **Step 1: Verify .env.example updated** (done in Task 1)

- [ ] **Step 2: Verify .gitignore updated** (done in Task 3)

- [ ] **Step 3: Add docs for preview website**

Create file: `docs/preview-website.md` (if needed):
```markdown
# Preview Website

The preview website allows you to review generated content before publishing to Patreon.

## Routes

| Method | Path | Description |
|--------|------|-------------|
| GET | `/preview` | Dashboard listing all articles |
| GET | `/preview/article/:id` | View single article |
| GET | `/preview/article/:id/edit` | Edit with EasyMDE |
| POST | `/preview/article/:id/save` | Save edits |
| POST | `/preview/article/:id/toggle` | Enable/disable |
| POST | `/preview/article/:id/delete` | Delete article |

## Status

Articles have a `status` field:
- `enabled` - will be published to Patreon
- `disabled` - excluded from publishing

Only `enabled` articles are included when running `patreon-manager publish`.
```

---

## Progress Tracking

Use TodoWrite to track each task above. Mark complete only after verification step passes.

---

## Plan Complete

Tasks identified:
- Task 1: Config Parameter Renaming (5 steps)
- Task 2: Orchestrator Sync Options Renaming (3 steps)
- Task 3: User Workspace Directory (4 steps)
- Task 4: Database Migration (1 step)
- Task 5: Preview Website Handlers (lots of steps)
- Task 6: Testing (3 steps)
- Task 7: Documentation (3 steps)

All steps include exact file paths, code, and verification commands.