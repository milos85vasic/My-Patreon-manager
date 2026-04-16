# Repository Filtering, User Workspace & Preview Website — Design Spec

**Date:** 2026-04-16  
**Author:** opencode (deepseek-reasoner)  
**Status:** User Approved

## 1 — Configuration Parameters

### 1.1 Parameter Renaming (Breaking Change)

| Old Parameter | New Parameter | Default | Notes |
|---------------|---------------|----------|-------|
| `PROCESS_PRIVATE_REPOS` | `PROCESS_PRIVATE_REPOSITORIES` | `false` | Boolean — include private repos |
| `REPO_MAX_INACTIVITY_DAYS` | `MIN_MONTHS_COMMIT_ACTIVITY` | `18` | Months — repos with commits in last N months |

### 1.2 New Parameters

| Parameter | Default | Notes |
|-----------|---------|-------|
| `USER_WORKSPACE_DIR` | `user` | Root directory for all user data |
| `DB_PATH` default | `user/db/patreon_manager.db` | SQLite in workspace |

### 1.3 Internal Struct Changes

```go
// Config fields renamed
type Config struct {
    // ...
    ProcessPrivateRepositories bool
    MinMonthsCommitActivity    int  // was RepoMaxInactivityDays
}

// Orchestrator fields renamed
type SyncOptions struct {
    // ...
    ProcessPrivateRepositories bool
    MinMonthsCommitActivity  int
}
```

### 1.4 Calculation Change

Old: `cutoff := time.Now().AddDate(0, 0, -opts.RepoMaxInactivityDays)`  
New: `cutoff := time.Now().AddDate(0, -opts.MinMonthsCommitActivity, 0)`

---

## 2 — User Workspace Directory

### 2.1 Directory Structure

```
user/
├── db/
│   └── patreon_manager.db    # SQLite database (default matric)
├── img/                      # Generated images / thumbnails
├── content/                  # Exported content files
└── templates/                # Custom content templates
```

### 2.2 Auto-Creation

On first run, the app automatically creates:
1. `user/` directory
2. `user/db/` subdirectory
3. `user/img/` subdirectory
4. `user/content/` subdirectory
5. `user/templates/` subdirectory
6. Empty SQLite database at `user/db/patreon_manager.db`

### 2.3 Gitignore

Add to `.gitignore`:
```
/user/
```

---

## 3 — Preview Website

### 3.1 Technology Stack

- **Backend**: Go html/template + embedded templates in existing Gin server
- **Frontend**: EasyMDE (CDN) for editing, responsive CSS

### 3.2 HTTP Routes

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| GET | `/preview` | Dashboard - list all articles | None |
| GET | `/preview/article/:id` | View single article | None |
| POST | `/preview/article/:id/delete` | Delete article | None |
| POST | `/preview/article/:id/toggle` | Enable/disable article | None |
| GET | `/preview/article/:id/edit` | Edit article (EasyMDE) | None |
| POST | `/preview/article/:id/save` | Save edited article | None |

### 3.3 Database Schema

Add to `generated_contents` table:
```sql
ALTER TABLE generated_contents ADD COLUMN status TEXT DEFAULT 'enabled';
-- Values: 'enabled', 'disabled'
-- Only status='enabled' articles get published to Patreon
```

### 3.4 Template Structure

```
internal/handlers/templates/preview/
├── layout.html    # Base HTML with nav + CSS
├── index.html     # Dashboard
├── article.html  # View article
└── edit.html      # Editor with EasyMDE
```

### 3.5 Features

**Dashboard (`/preview`):**
- List all generated content
- Show title, repository, creation date, status badge
- Actions: View, Edit, Toggle, Delete

**Toggle (`/preview/article/:id/toggle`):**
- POST toggles between 'enabled' ↔ 'disabled'
- Returns JSON `{success: true, status: "disabled"}`

**Edit (`/preview/article/:id/edit`):**
- EasyMDE loads with existing Markdown from `body` field
- Save button: POST to `/preview/article/:id/save`
- Cancel button: returns to article view

**Delete (`/preview/article/:id/delete`):**
- POST with confirm dialog
- Hard delete from DB (irreversible)
- Returns redirect to dashboard

---

## 4 — Testing Requirements

### 4.1 Unit Tests

| Area | Tests |
|------|-------|
| Config param loading | New param names load correctly |
| Config month→days conversion | 18 months → 548 days |
| Workspace auto-creation | All subdirectories created on first run |
| Repository filter | `MIN_MONTHS_COMMIT_ACTIVITY` filters correctly |
| Toggle endpoint | Status toggles enabled↔disabled |
| Save endpoint | Body field updates correctly |

### 4.2 Integration Tests

- `/preview/*` all 6 routes respond correctly
- Full toggle enable→disable→enable flow
- Full edit→save→view flow
- Database migration for new column

### 4.3 Test Categories

Expand these test categories per `scripts/coverage.sh`:
- **e2e**: Full pipeline with preview website
- **contract**: Preview endpoint interface compliance

---

## 5 — Documentation

### 5.1 Update Existing

| Doc | Changes |
|-----|---------|
| `.env.example` | New params, update comments, example values |
| `CLAUDE.md` | Note default DB path moved to `user/` |
| `AGENTS.md` | Same as CLAUDE.md |
| Configuration guide | Document `USER_WORKSPACE_DIR` |
| CLI sync/scan manual | Document new filtering params |

### 5.2 New Documentation

| Doc | Description |
|-----|-------------|
| Preview website guide | `/preview` routes, usage |
| User workspace setup | First-run creation, gitignore |

### 5.3 Video Course

- New episode: User workspace setup
- New episode: Preview website demonstration

---

## 6 — Implementation Order

1. **Config changes** — Rename params, add new workspace config
2. **Workspace auto-creation** — Create directories on first run
3. **Database migration** — Add `status` column to `generated_contents`
4. **Preview handlers** — Implement all 6 routes + templates
5. **Testing** — Unit + integration tests
6. **Documentation** — Update existing + new guides
7. **Video updates** — Record new episodes

---

## 7 — Success Criteria

- [ ] `PROCESS_PRIVATE_REPOSITORIES=false` works (old param deprecated)
- [ ] `MIN_MONTHS_COMMIT_ACTIVITY=18` filters repos (old param deprecated)
- [ ] First run creates `user/` with all subdirectories
- [ ] `user/` is gitignored
- [ ] DB defaults to `user/db/patreon_manager.db`
- [ ] `/preview` shows all articles
- [ ] Toggle enables/disables articles
- [ ] Edit saves changes to DB
- [ ] Delete removes article from DB
- [ ] 100% test coverage maintained
- [ ] Documentation updated

---

## 8 — Related Files

- `internal/config/config.go` — Config struct + loading
- `internal/services/sync/orchestrator.go` — Filter logic
- `internal/database/sqlite.go` — Migration
- `internal/handlers/` — Preview HTTP handlers
- `.env.example` — Parameter docs
- `.gitignore` — Ignore `user/`