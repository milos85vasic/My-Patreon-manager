package database

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

type SQLiteRepositoryStore struct {
	db *sql.DB
}

func (s *SQLiteRepositoryStore) Create(ctx context.Context, repo *models.Repository) error {
	topics, _ := json.Marshal(repo.Topics)
	langStats, _ := json.Marshal(repo.LanguageStats)
	_, err := s.db.ExecContext(ctx, `INSERT INTO repositories (id, service, owner, name, url, https_url, description, readme_content, readme_format, topics, primary_language, language_stats, stars, forks, last_commit_sha, last_commit_at, is_archived, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		repo.ID, repo.Service, repo.Owner, repo.Name, repo.URL, repo.HTTPSURL, repo.Description, repo.READMEContent, repo.READMEFormat, string(topics), repo.PrimaryLanguage, string(langStats), repo.Stars, repo.Forks, repo.LastCommitSHA, repo.LastCommitAt, repo.IsArchived, repo.CreatedAt, repo.UpdatedAt)
	return err
}

func (s *SQLiteRepositoryStore) GetByID(ctx context.Context, id string) (*models.Repository, error) {
	repo := &models.Repository{}
	var topics, langStats []byte
	err := s.db.QueryRowContext(ctx, "SELECT id, service, owner, name, url, https_url, description, readme_content, readme_format, topics, primary_language, language_stats, stars, forks, last_commit_sha, last_commit_at, is_archived, created_at, updated_at FROM repositories WHERE id = ?", id).Scan(&repo.ID, &repo.Service, &repo.Owner, &repo.Name, &repo.URL, &repo.HTTPSURL, &repo.Description, &repo.READMEContent, &repo.READMEFormat, &topics, &repo.PrimaryLanguage, &langStats, &repo.Stars, &repo.Forks, &repo.LastCommitSHA, &repo.LastCommitAt, &repo.IsArchived, &repo.CreatedAt, &repo.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	json.Unmarshal(topics, &repo.Topics)
	json.Unmarshal(langStats, &repo.LanguageStats)
	return repo, nil
}

func (s *SQLiteRepositoryStore) GetByServiceOwnerName(ctx context.Context, service, owner, name string) (*models.Repository, error) {
	repo := &models.Repository{}
	var topics, langStats []byte
	err := s.db.QueryRowContext(ctx, "SELECT id, service, owner, name, url, https_url, description, readme_content, readme_format, topics, primary_language, language_stats, stars, forks, last_commit_sha, last_commit_at, is_archived, created_at, updated_at FROM repositories WHERE service=? AND owner=? AND name=?", service, owner, name).Scan(&repo.ID, &repo.Service, &repo.Owner, &repo.Name, &repo.URL, &repo.HTTPSURL, &repo.Description, &repo.READMEContent, &repo.READMEFormat, &topics, &repo.PrimaryLanguage, &langStats, &repo.Stars, &repo.Forks, &repo.LastCommitSHA, &repo.LastCommitAt, &repo.IsArchived, &repo.CreatedAt, &repo.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	json.Unmarshal(topics, &repo.Topics)
	json.Unmarshal(langStats, &repo.LanguageStats)
	return repo, nil
}

func (s *SQLiteRepositoryStore) List(ctx context.Context, filter RepositoryFilter) ([]*models.Repository, error) {
	query := "SELECT id, service, owner, name, url, https_url, description, readme_content, readme_format, topics, primary_language, language_stats, stars, forks, last_commit_sha, last_commit_at, is_archived, created_at, updated_at FROM repositories WHERE 1=1"
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
		repo := &models.Repository{}
		var topics, langStats []byte
		if err := rows.Scan(&repo.ID, &repo.Service, &repo.Owner, &repo.Name, &repo.URL, &repo.HTTPSURL, &repo.Description, &repo.READMEContent, &repo.READMEFormat, &topics, &repo.PrimaryLanguage, &langStats, &repo.Stars, &repo.Forks, &repo.LastCommitSHA, &repo.LastCommitAt, &repo.IsArchived, &repo.CreatedAt, &repo.UpdatedAt); err != nil {
			return nil, err
		}
		json.Unmarshal(topics, &repo.Topics)
		json.Unmarshal(langStats, &repo.LanguageStats)
		repos = append(repos, repo)
	}
	return repos, nil
}

func (s *SQLiteRepositoryStore) Update(ctx context.Context, repo *models.Repository) error {
	topics, _ := json.Marshal(repo.Topics)
	langStats, _ := json.Marshal(repo.LanguageStats)
	_, err := s.db.ExecContext(ctx, "UPDATE repositories SET service=?, owner=?, name=?, url=?, https_url=?, description=?, readme_content=?, readme_format=?, topics=?, primary_language=?, language_stats=?, stars=?, forks=?, last_commit_sha=?, last_commit_at=?, is_archived=?, updated_at=CURRENT_TIMESTAMP WHERE id=?",
		repo.Service, repo.Owner, repo.Name, repo.URL, repo.HTTPSURL, repo.Description, repo.READMEContent, repo.READMEFormat, string(topics), repo.PrimaryLanguage, string(langStats), repo.Stars, repo.Forks, repo.LastCommitSHA, repo.LastCommitAt, repo.IsArchived, repo.ID)
	return err
}

func (s *SQLiteRepositoryStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM repositories WHERE id=?", id)
	return err
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
