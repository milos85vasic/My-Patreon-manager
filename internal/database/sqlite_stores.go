package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

type SQLiteRepositoryStore struct {
	db *sql.DB
}

// repoColumnList lists every column returned by repository SELECTs. The
// column order must match scanRepository below.
const sqliteRepoColumnList = `id, service, owner, name, url, https_url, description, readme_content, readme_format, topics, primary_language, language_stats, stars, forks, last_commit_sha, last_commit_at, is_archived, created_at, updated_at, current_revision_id, published_revision_id, process_state, last_processed_at`

// scanSQLiteRepository scans a single row into a *models.Repository. It
// reads the two nullable TEXT columns (current_revision_id,
// published_revision_id, last_processed_at) via sql.NullString / sql.NullTime
// so TEXT-affinity storage on SQLite works uniformly.
func scanSQLiteRepository(scan func(dest ...interface{}) error) (*models.Repository, error) {
	repo := &models.Repository{}
	var topics, langStats []byte
	var currentRev, publishedRev sql.NullString
	var lastProcessed sql.NullString
	var lastCommitAt sql.NullTime
	if err := scan(&repo.ID, &repo.Service, &repo.Owner, &repo.Name, &repo.URL, &repo.HTTPSURL,
		&repo.Description, &repo.READMEContent, &repo.READMEFormat,
		&topics, &repo.PrimaryLanguage, &langStats,
		&repo.Stars, &repo.Forks, &repo.LastCommitSHA, &lastCommitAt, &repo.IsArchived,
		&repo.CreatedAt, &repo.UpdatedAt,
		&currentRev, &publishedRev, &repo.ProcessState, &lastProcessed); err != nil {
		return nil, err
	}
	_ = json.Unmarshal(topics, &repo.Topics)
	_ = json.Unmarshal(langStats, &repo.LanguageStats)
	if lastCommitAt.Valid {
		repo.LastCommitAt = lastCommitAt.Time
	}
	if currentRev.Valid && currentRev.String != "" {
		s := currentRev.String
		repo.CurrentRevisionID = &s
	}
	if publishedRev.Valid && publishedRev.String != "" {
		s := publishedRev.String
		repo.PublishedRevisionID = &s
	}
	if lastProcessed.Valid && lastProcessed.String != "" {
		t, err := parseTimeString(lastProcessed.String)
		if err != nil {
			return nil, err
		}
		if !t.IsZero() {
			repo.LastProcessedAt = &t
		}
	}
	return repo, nil
}

func (s *SQLiteRepositoryStore) Create(ctx context.Context, repo *models.Repository) error {
	topics, _ := json.Marshal(repo.Topics)
	langStats, _ := json.Marshal(repo.LanguageStats)
	processState := repo.ProcessState
	if processState == "" {
		processState = "idle"
	}
	var currentRev, publishedRev interface{}
	if repo.CurrentRevisionID != nil {
		currentRev = *repo.CurrentRevisionID
	}
	if repo.PublishedRevisionID != nil {
		publishedRev = *repo.PublishedRevisionID
	}
	var lastProcessed interface{}
	if repo.LastProcessedAt != nil {
		lastProcessed = formatTime(*repo.LastProcessedAt)
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO repositories (id, service, owner, name, url, https_url, description, readme_content, readme_format, topics, primary_language, language_stats, stars, forks, last_commit_sha, last_commit_at, is_archived, created_at, updated_at, current_revision_id, published_revision_id, process_state, last_processed_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		repo.ID, repo.Service, repo.Owner, repo.Name, repo.URL, repo.HTTPSURL, repo.Description, repo.READMEContent, repo.READMEFormat, string(topics), repo.PrimaryLanguage, string(langStats), repo.Stars, repo.Forks, repo.LastCommitSHA, repo.LastCommitAt, repo.IsArchived, repo.CreatedAt, repo.UpdatedAt, currentRev, publishedRev, processState, lastProcessed)
	return err
}

func (s *SQLiteRepositoryStore) GetByID(ctx context.Context, id string) (*models.Repository, error) {
	row := s.db.QueryRowContext(ctx, "SELECT "+sqliteRepoColumnList+" FROM repositories WHERE id = ?", id)
	repo, err := scanSQLiteRepository(row.Scan)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return repo, nil
}

func (s *SQLiteRepositoryStore) GetByServiceOwnerName(ctx context.Context, service, owner, name string) (*models.Repository, error) {
	row := s.db.QueryRowContext(ctx, "SELECT "+sqliteRepoColumnList+" FROM repositories WHERE service=? AND owner=? AND name=?", service, owner, name)
	repo, err := scanSQLiteRepository(row.Scan)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return repo, nil
}

func (s *SQLiteRepositoryStore) List(ctx context.Context, filter RepositoryFilter) ([]*models.Repository, error) {
	query := "SELECT " + sqliteRepoColumnList + " FROM repositories WHERE 1=1"
	args := []interface{}{}
	if filter.Service != "" {
		query += " AND service=?"
		args = append(args, filter.Service)
	}
	if filter.Owner != "" {
		query += " AND owner=?"
		args = append(args, filter.Owner)
	}
	if filter.IsArchived != nil {
		query += " AND is_archived=?"
		args = append(args, *filter.IsArchived)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var repos []*models.Repository
	for rows.Next() {
		repo, err := scanSQLiteRepository(rows.Scan)
		if err != nil {
			return nil, err
		}
		repos = append(repos, repo)
	}
	return repos, nil
}

func (s *SQLiteRepositoryStore) Update(ctx context.Context, repo *models.Repository) error {
	topics, _ := json.Marshal(repo.Topics)
	langStats, _ := json.Marshal(repo.LanguageStats)
	processState := repo.ProcessState
	if processState == "" {
		processState = "idle"
	}
	var currentRev, publishedRev interface{}
	if repo.CurrentRevisionID != nil {
		currentRev = *repo.CurrentRevisionID
	}
	if repo.PublishedRevisionID != nil {
		publishedRev = *repo.PublishedRevisionID
	}
	var lastProcessed interface{}
	if repo.LastProcessedAt != nil {
		lastProcessed = formatTime(*repo.LastProcessedAt)
	}
	_, err := s.db.ExecContext(ctx, "UPDATE repositories SET service=?, owner=?, name=?, url=?, https_url=?, description=?, readme_content=?, readme_format=?, topics=?, primary_language=?, language_stats=?, stars=?, forks=?, last_commit_sha=?, last_commit_at=?, is_archived=?, current_revision_id=?, published_revision_id=?, process_state=?, last_processed_at=?, updated_at=CURRENT_TIMESTAMP WHERE id=?",
		repo.Service, repo.Owner, repo.Name, repo.URL, repo.HTTPSURL, repo.Description, repo.READMEContent, repo.READMEFormat, string(topics), repo.PrimaryLanguage, string(langStats), repo.Stars, repo.Forks, repo.LastCommitSHA, repo.LastCommitAt, repo.IsArchived, currentRev, publishedRev, processState, lastProcessed, repo.ID)
	return err
}

func (s *SQLiteRepositoryStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM repositories WHERE id=?", id)
	return err
}

// SetRevisionPointers updates current_revision_id and (conditionally)
// published_revision_id. An empty publishedID leaves published_revision_id
// unchanged; a non-empty value overwrites it. The currentID pointer is
// always written, even if empty.
func (s *SQLiteRepositoryStore) SetRevisionPointers(ctx context.Context, repoID, currentID, publishedID string) error {
	if publishedID == "" {
		_, err := s.db.ExecContext(ctx,
			"UPDATE repositories SET current_revision_id=? WHERE id=?",
			currentID, repoID)
		return err
	}
	_, err := s.db.ExecContext(ctx,
		"UPDATE repositories SET current_revision_id=?, published_revision_id=? WHERE id=?",
		currentID, publishedID, repoID)
	return err
}

// SetProcessState overwrites the process_state column.
func (s *SQLiteRepositoryStore) SetProcessState(ctx context.Context, repoID, state string) error {
	_, err := s.db.ExecContext(ctx,
		"UPDATE repositories SET process_state=? WHERE id=?",
		state, repoID)
	return err
}

// SetLastProcessedAt overwrites last_processed_at, stamping it with the
// given time (serialized via formatTime for uniform parsing on the way back).
func (s *SQLiteRepositoryStore) SetLastProcessedAt(ctx context.Context, repoID string, t time.Time) error {
	_, err := s.db.ExecContext(ctx,
		"UPDATE repositories SET last_processed_at=? WHERE id=?",
		formatTime(t), repoID)
	return err
}

// ListForProcessQueue returns every repository row ordered for fair
// queueing: NULL last_processed_at first (never-processed repos),
// then older timestamps ahead of newer ones, with id ASC as a stable
// tiebreaker. No filtering is applied here; the process-command queue
// builder decides which rows to skip.
//
// SQLite trick: `last_processed_at IS NULL` evaluates to 1 when NULL
// and 0 otherwise, so `ORDER BY last_processed_at IS NULL DESC` puts
// NULLs first without relying on the NULLS FIRST extension.
func (s *SQLiteRepositoryStore) ListForProcessQueue(ctx context.Context) ([]*models.Repository, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+sqliteRepoColumnList+" FROM repositories ORDER BY last_processed_at IS NULL DESC, last_processed_at ASC, id ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var repos []*models.Repository
	for rows.Next() {
		repo, err := scanSQLiteRepository(rows.Scan)
		if err != nil {
			return nil, err
		}
		repos = append(repos, repo)
	}
	return repos, nil
}

type SQLiteSyncStateStore struct {
	db *sql.DB
}

func (s *SQLiteSyncStateStore) Create(ctx context.Context, state *models.SyncState) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO sync_states (id, repository_id, patreon_post_id, last_sync_at, last_commit_sha, last_content_hash, status, last_failure_reason, grace_period_until, checkpoint, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		state.ID, state.RepositoryID, state.PatreonPostID, state.LastSyncAt, state.LastCommitSHA, state.LastContentHash, state.Status, state.LastFailureReason, state.GracePeriodUntil, state.Checkpoint, state.CreatedAt, state.UpdatedAt)
	return err
}

func (s *SQLiteSyncStateStore) GetByID(ctx context.Context, id string) (*models.SyncState, error) {
	st := &models.SyncState{}
	err := s.db.QueryRowContext(ctx, "SELECT id, repository_id, patreon_post_id, last_sync_at, last_commit_sha, last_content_hash, status, last_failure_reason, grace_period_until, checkpoint, created_at, updated_at FROM sync_states WHERE id=?", id).Scan(&st.ID, &st.RepositoryID, &st.PatreonPostID, &st.LastSyncAt, &st.LastCommitSHA, &st.LastContentHash, &st.Status, &st.LastFailureReason, &st.GracePeriodUntil, &st.Checkpoint, &st.CreatedAt, &st.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return st, err
}

func (s *SQLiteSyncStateStore) GetByRepositoryID(ctx context.Context, repoID string) (*models.SyncState, error) {
	st := &models.SyncState{}
	err := s.db.QueryRowContext(ctx, "SELECT id, repository_id, patreon_post_id, last_sync_at, last_commit_sha, last_content_hash, status, last_failure_reason, grace_period_until, checkpoint, created_at, updated_at FROM sync_states WHERE repository_id=?", repoID).Scan(&st.ID, &st.RepositoryID, &st.PatreonPostID, &st.LastSyncAt, &st.LastCommitSHA, &st.LastContentHash, &st.Status, &st.LastFailureReason, &st.GracePeriodUntil, &st.Checkpoint, &st.CreatedAt, &st.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return st, err
}

func (s *SQLiteSyncStateStore) GetByStatus(ctx context.Context, status string) ([]*models.SyncState, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, repository_id, patreon_post_id, last_sync_at, last_commit_sha, last_content_hash, status, last_failure_reason, grace_period_until, checkpoint, created_at, updated_at FROM sync_states WHERE status=?", status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var states []*models.SyncState
	for rows.Next() {
		st := &models.SyncState{}
		if err := rows.Scan(&st.ID, &st.RepositoryID, &st.PatreonPostID, &st.LastSyncAt, &st.LastCommitSHA, &st.LastContentHash, &st.Status, &st.LastFailureReason, &st.GracePeriodUntil, &st.Checkpoint, &st.CreatedAt, &st.UpdatedAt); err != nil {
			return nil, err
		}
		states = append(states, st)
	}
	return states, nil
}

func (s *SQLiteSyncStateStore) UpdateStatus(ctx context.Context, repoID, status, reason string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE sync_states SET status=?, last_failure_reason=?, updated_at=CURRENT_TIMESTAMP WHERE repository_id=?", status, reason, repoID)
	return err
}

func (s *SQLiteSyncStateStore) UpdateCheckpoint(ctx context.Context, repoID, checkpoint string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE sync_states SET checkpoint=?, updated_at=CURRENT_TIMESTAMP WHERE repository_id=?", checkpoint, repoID)
	return err
}

func (s *SQLiteSyncStateStore) Update(ctx context.Context, state *models.SyncState) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sync_states SET repository_id=?, patreon_post_id=?, last_sync_at=?, last_commit_sha=?, last_content_hash=?, status=?, last_failure_reason=?, grace_period_until=?, checkpoint=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		state.RepositoryID, state.PatreonPostID, state.LastSyncAt, state.LastCommitSHA, state.LastContentHash, state.Status, state.LastFailureReason, state.GracePeriodUntil, state.Checkpoint, state.ID)
	return err
}

func (s *SQLiteSyncStateStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM sync_states WHERE id=?", id)
	return err
}

type SQLiteMirrorMapStore struct {
	db *sql.DB
}

func (s *SQLiteMirrorMapStore) Create(ctx context.Context, m *models.MirrorMap) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO mirror_maps (id, mirror_group_id, repository_id, is_canonical, confidence_score, detection_method, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		m.ID, m.MirrorGroupID, m.RepositoryID, m.IsCanonical, m.ConfidenceScore, m.DetectionMethod, m.CreatedAt)
	return err
}

func (s *SQLiteMirrorMapStore) GetByMirrorGroupID(ctx context.Context, groupID string) ([]*models.MirrorMap, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, mirror_group_id, repository_id, is_canonical, confidence_score, detection_method, created_at FROM mirror_maps WHERE mirror_group_id=?", groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var maps []*models.MirrorMap
	for rows.Next() {
		m := &models.MirrorMap{}
		if err := rows.Scan(&m.ID, &m.MirrorGroupID, &m.RepositoryID, &m.IsCanonical, &m.ConfidenceScore, &m.DetectionMethod, &m.CreatedAt); err != nil {
			return nil, err
		}
		maps = append(maps, m)
	}
	return maps, nil
}

func (s *SQLiteMirrorMapStore) GetByRepositoryID(ctx context.Context, repoID string) ([]*models.MirrorMap, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, mirror_group_id, repository_id, is_canonical, confidence_score, detection_method, created_at FROM mirror_maps WHERE repository_id=?", repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var maps []*models.MirrorMap
	for rows.Next() {
		m := &models.MirrorMap{}
		if err := rows.Scan(&m.ID, &m.MirrorGroupID, &m.RepositoryID, &m.IsCanonical, &m.ConfidenceScore, &m.DetectionMethod, &m.CreatedAt); err != nil {
			return nil, err
		}
		maps = append(maps, m)
	}
	return maps, nil
}

func (s *SQLiteMirrorMapStore) GetAllGroups(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT DISTINCT mirror_group_id FROM mirror_maps")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var groups []string
	for rows.Next() {
		var g string
		if err := rows.Scan(&g); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return groups, nil
}

func (s *SQLiteMirrorMapStore) SetCanonical(ctx context.Context, repoID string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE mirror_maps SET is_canonical=0 WHERE mirror_group_id=(SELECT mirror_group_id FROM mirror_maps WHERE repository_id=?)", repoID)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, "UPDATE mirror_maps SET is_canonical=1 WHERE repository_id=?", repoID)
	return err
}

func (s *SQLiteMirrorMapStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM mirror_maps WHERE id=?", id)
	return err
}

func (s *SQLiteMirrorMapStore) DeleteAll(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM mirror_maps")
	return err
}

type SQLiteGeneratedContentStore struct {
	db *sql.DB
}

func (s *SQLiteGeneratedContentStore) Create(ctx context.Context, c *models.GeneratedContent) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO generated_contents (id, repository_id, content_type, format, title, body, quality_score, model_used, prompt_template, token_count, generation_attempts, passed_quality_gate, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.RepositoryID, c.ContentType, c.Format, c.Title, c.Body, c.QualityScore, c.ModelUsed, c.PromptTemplate, c.TokenCount, c.GenerationAttempts, c.PassedQualityGate, c.CreatedAt)
	return err
}

func (s *SQLiteGeneratedContentStore) GetByID(ctx context.Context, id string) (*models.GeneratedContent, error) {
	c := &models.GeneratedContent{}
	err := s.db.QueryRowContext(ctx, "SELECT id, repository_id, content_type, format, title, body, quality_score, model_used, prompt_template, token_count, generation_attempts, passed_quality_gate, created_at FROM generated_contents WHERE id=?", id).Scan(&c.ID, &c.RepositoryID, &c.ContentType, &c.Format, &c.Title, &c.Body, &c.QualityScore, &c.ModelUsed, &c.PromptTemplate, &c.TokenCount, &c.GenerationAttempts, &c.PassedQualityGate, &c.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return c, err
}

func (s *SQLiteGeneratedContentStore) GetLatestByRepo(ctx context.Context, repoID string) (*models.GeneratedContent, error) {
	c := &models.GeneratedContent{}
	err := s.db.QueryRowContext(ctx, "SELECT id, repository_id, content_type, format, title, body, quality_score, model_used, prompt_template, token_count, generation_attempts, passed_quality_gate, created_at FROM generated_contents WHERE repository_id=? ORDER BY created_at DESC LIMIT 1", repoID).Scan(&c.ID, &c.RepositoryID, &c.ContentType, &c.Format, &c.Title, &c.Body, &c.QualityScore, &c.ModelUsed, &c.PromptTemplate, &c.TokenCount, &c.GenerationAttempts, &c.PassedQualityGate, &c.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return c, err
}

func (s *SQLiteGeneratedContentStore) GetByQualityRange(ctx context.Context, min, max float64) ([]*models.GeneratedContent, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, repository_id, content_type, format, title, body, quality_score, model_used, prompt_template, token_count, generation_attempts, passed_quality_gate, created_at FROM generated_contents WHERE quality_score >= ? AND quality_score <= ?", min, max)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var contents []*models.GeneratedContent
	for rows.Next() {
		c := &models.GeneratedContent{}
		if err := rows.Scan(&c.ID, &c.RepositoryID, &c.ContentType, &c.Format, &c.Title, &c.Body, &c.QualityScore, &c.ModelUsed, &c.PromptTemplate, &c.TokenCount, &c.GenerationAttempts, &c.PassedQualityGate, &c.CreatedAt); err != nil {
			return nil, err
		}
		contents = append(contents, c)
	}
	return contents, nil
}

func (s *SQLiteGeneratedContentStore) ListByRepository(ctx context.Context, repoID string) ([]*models.GeneratedContent, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, repository_id, content_type, format, title, body, quality_score, model_used, prompt_template, token_count, generation_attempts, passed_quality_gate, created_at FROM generated_contents WHERE repository_id=? ORDER BY created_at DESC", repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var contents []*models.GeneratedContent
	for rows.Next() {
		c := &models.GeneratedContent{}
		if err := rows.Scan(&c.ID, &c.RepositoryID, &c.ContentType, &c.Format, &c.Title, &c.Body, &c.QualityScore, &c.ModelUsed, &c.PromptTemplate, &c.TokenCount, &c.GenerationAttempts, &c.PassedQualityGate, &c.CreatedAt); err != nil {
			return nil, err
		}
		contents = append(contents, c)
	}
	return contents, nil
}

func (s *SQLiteGeneratedContentStore) Update(ctx context.Context, c *models.GeneratedContent) error {
	_, err := s.db.ExecContext(ctx, `UPDATE generated_contents SET repository_id=?, content_type=?, format=?, title=?, body=?, quality_score=?, model_used=?, prompt_template=?, token_count=?, generation_attempts=?, passed_quality_gate=?, created_at=? WHERE id=?`,
		c.RepositoryID, c.ContentType, c.Format, c.Title, c.Body, c.QualityScore, c.ModelUsed, c.PromptTemplate, c.TokenCount, c.GenerationAttempts, c.PassedQualityGate, c.CreatedAt, c.ID)
	return err
}

type SQLiteContentTemplateStore struct {
	db *sql.DB
}

func (s *SQLiteContentTemplateStore) Create(ctx context.Context, t *models.ContentTemplate) error {
	vars, _ := json.Marshal(t.Variables)
	_, err := s.db.ExecContext(ctx, `INSERT INTO content_templates (id, name, content_type, language, template, variables, min_length, max_length, quality_tier, is_built_in, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Name, t.ContentType, t.Language, t.Template, string(vars), t.MinLength, t.MaxLength, t.QualityTier, t.IsBuiltIn, t.CreatedAt, t.UpdatedAt)
	return err
}

func (s *SQLiteContentTemplateStore) GetByName(ctx context.Context, name string) (*models.ContentTemplate, error) {
	t := &models.ContentTemplate{}
	var vars []byte
	err := s.db.QueryRowContext(ctx, "SELECT id, name, content_type, language, template, variables, min_length, max_length, quality_tier, is_built_in, created_at, updated_at FROM content_templates WHERE name=?", name).Scan(&t.ID, &t.Name, &t.ContentType, &t.Language, &t.Template, &vars, &t.MinLength, &t.MaxLength, &t.QualityTier, &t.IsBuiltIn, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	json.Unmarshal(vars, &t.Variables)
	return t, nil
}

func (s *SQLiteContentTemplateStore) ListByContentType(ctx context.Context, contentType string) ([]*models.ContentTemplate, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, name, content_type, language, template, variables, min_length, max_length, quality_tier, is_built_in, created_at, updated_at FROM content_templates WHERE content_type=?", contentType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var templates []*models.ContentTemplate
	for rows.Next() {
		t := &models.ContentTemplate{}
		var vars []byte
		if err := rows.Scan(&t.ID, &t.Name, &t.ContentType, &t.Language, &t.Template, &vars, &t.MinLength, &t.MaxLength, &t.QualityTier, &t.IsBuiltIn, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		json.Unmarshal(vars, &t.Variables)
		templates = append(templates, t)
	}
	return templates, nil
}

func (s *SQLiteContentTemplateStore) Update(ctx context.Context, t *models.ContentTemplate) error {
	vars, _ := json.Marshal(t.Variables)
	_, err := s.db.ExecContext(ctx, "UPDATE content_templates SET name=?, content_type=?, language=?, template=?, variables=?, min_length=?, max_length=?, quality_tier=?, is_built_in=?, updated_at=CURRENT_TIMESTAMP WHERE id=?",
		t.Name, t.ContentType, t.Language, t.Template, string(vars), t.MinLength, t.MaxLength, t.QualityTier, t.IsBuiltIn, t.ID)
	return err
}

func (s *SQLiteContentTemplateStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM content_templates WHERE id=?", id)
	return err
}

type SQLitePostStore struct {
	db *sql.DB
}

func (s *SQLitePostStore) Create(ctx context.Context, p *models.Post) error {
	tierIDs, _ := json.Marshal(p.TierIDs)
	_, err := s.db.ExecContext(ctx, `INSERT INTO posts (id, campaign_id, repository_id, title, content, post_type, tier_ids, publication_status, published_at, is_manually_edited, content_hash, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.CampaignID, p.RepositoryID, p.Title, p.Content, p.PostType, string(tierIDs), p.PublicationStatus, p.PublishedAt, p.IsManuallyEdited, p.ContentHash, p.CreatedAt, p.UpdatedAt)
	return err
}

func (s *SQLitePostStore) GetByID(ctx context.Context, id string) (*models.Post, error) {
	p := &models.Post{}
	var tierIDs []byte
	err := s.db.QueryRowContext(ctx, "SELECT id, campaign_id, repository_id, title, content, post_type, tier_ids, publication_status, published_at, is_manually_edited, content_hash, created_at, updated_at FROM posts WHERE id=?", id).Scan(&p.ID, &p.CampaignID, &p.RepositoryID, &p.Title, &p.Content, &p.PostType, &tierIDs, &p.PublicationStatus, &p.PublishedAt, &p.IsManuallyEdited, &p.ContentHash, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	json.Unmarshal(tierIDs, &p.TierIDs)
	return p, nil
}

func (s *SQLitePostStore) GetByRepositoryID(ctx context.Context, repoID string) (*models.Post, error) {
	p := &models.Post{}
	var tierIDs []byte
	err := s.db.QueryRowContext(ctx, "SELECT id, campaign_id, repository_id, title, content, post_type, tier_ids, publication_status, published_at, is_manually_edited, content_hash, created_at, updated_at FROM posts WHERE repository_id=?", repoID).Scan(&p.ID, &p.CampaignID, &p.RepositoryID, &p.Title, &p.Content, &p.PostType, &tierIDs, &p.PublicationStatus, &p.PublishedAt, &p.IsManuallyEdited, &p.ContentHash, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	json.Unmarshal(tierIDs, &p.TierIDs)
	return p, nil
}

func (s *SQLitePostStore) Update(ctx context.Context, p *models.Post) error {
	tierIDs, _ := json.Marshal(p.TierIDs)
	_, err := s.db.ExecContext(ctx, `UPDATE posts SET campaign_id=?, repository_id=?, title=?, content=?, post_type=?, tier_ids=?, publication_status=?, published_at=?, is_manually_edited=?, content_hash=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		p.CampaignID, p.RepositoryID, p.Title, p.Content, p.PostType, string(tierIDs), p.PublicationStatus, p.PublishedAt, p.IsManuallyEdited, p.ContentHash, p.ID)
	return err
}

func (s *SQLitePostStore) UpdatePublicationStatus(ctx context.Context, id, status string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE posts SET publication_status=?, updated_at=CURRENT_TIMESTAMP WHERE id=?", status, id)
	return err
}

func (s *SQLitePostStore) MarkManuallyEdited(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE posts SET is_manually_edited=1, updated_at=CURRENT_TIMESTAMP WHERE id=?", id)
	return err
}

func (s *SQLitePostStore) ListByStatus(ctx context.Context, status string) ([]*models.Post, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, campaign_id, repository_id, title, content, post_type, tier_ids, publication_status, published_at, is_manually_edited, content_hash, created_at, updated_at FROM posts WHERE publication_status=?", status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var posts []*models.Post
	for rows.Next() {
		p := &models.Post{}
		var tierIDs []byte
		if err := rows.Scan(&p.ID, &p.CampaignID, &p.RepositoryID, &p.Title, &p.Content, &p.PostType, &tierIDs, &p.PublicationStatus, &p.PublishedAt, &p.IsManuallyEdited, &p.ContentHash, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		json.Unmarshal(tierIDs, &p.TierIDs)
		posts = append(posts, p)
	}
	return posts, nil
}

func (s *SQLitePostStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM posts WHERE id=?", id)
	return err
}

type SQLiteAuditEntryStore struct {
	db *sql.DB
}

func (s *SQLiteAuditEntryStore) Create(ctx context.Context, e *models.AuditEntry) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO audit_entries (id, repository_id, event_type, source_state, generation_params, publication_meta, actor, outcome, error_message, timestamp) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.RepositoryID, e.EventType, e.SourceState, e.GenerationParams, e.PublicationMeta, e.Actor, e.Outcome, e.ErrorMessage, e.Timestamp)
	return err
}

func (s *SQLiteAuditEntryStore) ListByRepository(ctx context.Context, repoID string) ([]*models.AuditEntry, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, repository_id, event_type, source_state, generation_params, publication_meta, actor, outcome, error_message, timestamp FROM audit_entries WHERE repository_id=? ORDER BY timestamp DESC", repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []*models.AuditEntry
	for rows.Next() {
		e := &models.AuditEntry{}
		if err := rows.Scan(&e.ID, &e.RepositoryID, &e.EventType, &e.SourceState, &e.GenerationParams, &e.PublicationMeta, &e.Actor, &e.Outcome, &e.ErrorMessage, &e.Timestamp); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, nil
}

func (s *SQLiteAuditEntryStore) ListByEventType(ctx context.Context, eventType string) ([]*models.AuditEntry, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, repository_id, event_type, source_state, generation_params, publication_meta, actor, outcome, error_message, timestamp FROM audit_entries WHERE event_type=? ORDER BY timestamp DESC", eventType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []*models.AuditEntry
	for rows.Next() {
		e := &models.AuditEntry{}
		if err := rows.Scan(&e.ID, &e.RepositoryID, &e.EventType, &e.SourceState, &e.GenerationParams, &e.PublicationMeta, &e.Actor, &e.Outcome, &e.ErrorMessage, &e.Timestamp); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, nil
}

func (s *SQLiteAuditEntryStore) ListByTimeRange(ctx context.Context, from, to string) ([]*models.AuditEntry, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, repository_id, event_type, source_state, generation_params, publication_meta, actor, outcome, error_message, timestamp FROM audit_entries WHERE timestamp >= ? AND timestamp <= ? ORDER BY timestamp DESC", from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []*models.AuditEntry
	for rows.Next() {
		e := &models.AuditEntry{}
		if err := rows.Scan(&e.ID, &e.RepositoryID, &e.EventType, &e.SourceState, &e.GenerationParams, &e.PublicationMeta, &e.Actor, &e.Outcome, &e.ErrorMessage, &e.Timestamp); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, nil
}

func (s *SQLiteAuditEntryStore) PurgeOlderThan(ctx context.Context, cutoff string) (int64, error) {
	result, err := s.db.ExecContext(ctx, "DELETE FROM audit_entries WHERE timestamp < ?", cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (db *SQLiteDB) Connect2(ctx context.Context, dsn string) error {
	return db.Connect(ctx, dsn)
}

type SQLiteIllustrationStore struct {
	db *sql.DB
}

func (s *SQLiteIllustrationStore) Create(ctx context.Context, ill *models.Illustration) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO illustrations (id, generated_content_id, repository_id, file_path, image_url, prompt, style, provider_used, format, size, content_hash, fingerprint, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ill.ID, ill.GeneratedContentID, ill.RepositoryID, ill.FilePath, ill.ImageURL, ill.Prompt, ill.Style, ill.ProviderUsed, ill.Format, ill.Size, ill.ContentHash, ill.Fingerprint, ill.CreatedAt)
	return err
}

func (s *SQLiteIllustrationStore) GetByID(ctx context.Context, id string) (*models.Illustration, error) {
	ill := &models.Illustration{}
	err := s.db.QueryRowContext(ctx,
		"SELECT id, generated_content_id, repository_id, file_path, image_url, prompt, style, provider_used, format, size, content_hash, fingerprint, created_at FROM illustrations WHERE id = ?",
		id).Scan(&ill.ID, &ill.GeneratedContentID, &ill.RepositoryID, &ill.FilePath, &ill.ImageURL, &ill.Prompt, &ill.Style, &ill.ProviderUsed, &ill.Format, &ill.Size, &ill.ContentHash, &ill.Fingerprint, &ill.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return ill, nil
}

func (s *SQLiteIllustrationStore) GetByContentID(ctx context.Context, contentID string) (*models.Illustration, error) {
	ill := &models.Illustration{}
	err := s.db.QueryRowContext(ctx,
		"SELECT id, generated_content_id, repository_id, file_path, image_url, prompt, style, provider_used, format, size, content_hash, fingerprint, created_at FROM illustrations WHERE generated_content_id = ?",
		contentID).Scan(&ill.ID, &ill.GeneratedContentID, &ill.RepositoryID, &ill.FilePath, &ill.ImageURL, &ill.Prompt, &ill.Style, &ill.ProviderUsed, &ill.Format, &ill.Size, &ill.ContentHash, &ill.Fingerprint, &ill.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return ill, nil
}

func (s *SQLiteIllustrationStore) GetByFingerprint(ctx context.Context, fingerprint string) (*models.Illustration, error) {
	ill := &models.Illustration{}
	err := s.db.QueryRowContext(ctx,
		"SELECT id, generated_content_id, repository_id, file_path, image_url, prompt, style, provider_used, format, size, content_hash, fingerprint, created_at FROM illustrations WHERE fingerprint = ?",
		fingerprint).Scan(&ill.ID, &ill.GeneratedContentID, &ill.RepositoryID, &ill.FilePath, &ill.ImageURL, &ill.Prompt, &ill.Style, &ill.ProviderUsed, &ill.Format, &ill.Size, &ill.ContentHash, &ill.Fingerprint, &ill.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return ill, nil
}

func (s *SQLiteIllustrationStore) ListByRepository(ctx context.Context, repoID string) ([]*models.Illustration, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, generated_content_id, repository_id, file_path, image_url, prompt, style, provider_used, format, size, content_hash, fingerprint, created_at FROM illustrations WHERE repository_id = ?",
		repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*models.Illustration
	for rows.Next() {
		ill := &models.Illustration{}
		if err := rows.Scan(&ill.ID, &ill.GeneratedContentID, &ill.RepositoryID, &ill.FilePath, &ill.ImageURL, &ill.Prompt, &ill.Style, &ill.ProviderUsed, &ill.Format, &ill.Size, &ill.ContentHash, &ill.Fingerprint, &ill.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, ill)
	}
	return result, nil
}

func (s *SQLiteIllustrationStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM illustrations WHERE id = ?", id)
	return err
}
