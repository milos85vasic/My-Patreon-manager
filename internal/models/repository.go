package models

import (
	"encoding/json"
	"time"
)

type Repository struct {
	ID              string             `json:"id" db:"id"`
	Service         string             `json:"service" db:"service"`
	Owner           string             `json:"owner" db:"owner"`
	Name            string             `json:"name" db:"name"`
	URL             string             `json:"url" db:"url"`
	HTTPSURL        string             `json:"https_url" db:"https_url"`
	Description     string             `json:"description" db:"description"`
	READMEContent   string             `json:"readme_content" db:"readme_content"`
	READMEFormat    string             `json:"readme_format" db:"readme_format"`
	Topics          []string           `json:"topics" db:"topics"`
	PrimaryLanguage string             `json:"primary_language" db:"primary_language"`
	LanguageStats   map[string]float64 `json:"language_stats" db:"language_stats"`
	Stars           int                `json:"stars" db:"stars"`
	Forks           int                `json:"forks" db:"forks"`
	LastCommitSHA   string             `json:"last_commit_sha" db:"last_commit_sha"`
	LastCommitAt    time.Time          `json:"last_commit_at" db:"last_commit_at"`
	IsArchived      bool               `json:"is_archived" db:"is_archived"`
	CreatedAt       time.Time          `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time          `json:"updated_at" db:"updated_at"`
}

type SyncState struct {
	ID                string    `json:"id" db:"id"`
	RepositoryID      string    `json:"repository_id" db:"repository_id"`
	PatreonPostID     string    `json:"patreon_post_id" db:"patreon_post_id"`
	LastSyncAt        time.Time `json:"last_sync_at" db:"last_sync_at"`
	LastCommitSHA     string    `json:"last_commit_sha" db:"last_commit_sha"`
	LastContentHash   string    `json:"last_content_hash" db:"last_content_hash"`
	Status            string    `json:"status" db:"status"`
	LastFailureReason string    `json:"last_failure_reason" db:"last_failure_reason"`
	GracePeriodUntil  time.Time `json:"grace_period_until" db:"grace_period_until"`
	Checkpoint        string    `json:"checkpoint" db:"checkpoint"`
	CreatedAt         time.Time `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time `json:"updated_at" db:"updated_at"`
}

type MirrorMap struct {
	ID              string    `json:"id" db:"id"`
	MirrorGroupID   string    `json:"mirror_group_id" db:"mirror_group_id"`
	RepositoryID    string    `json:"repository_id" db:"repository_id"`
	IsCanonical     bool      `json:"is_canonical" db:"is_canonical"`
	ConfidenceScore float64   `json:"confidence_score" db:"confidence_score"`
	DetectionMethod string    `json:"detection_method" db:"detection_method"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
}

type SyncLock struct {
	ID        string    `json:"id" db:"id"`
	PID       int       `json:"pid" db:"pid"`
	Hostname  string    `json:"hostname" db:"hostname"`
	StartedAt time.Time `json:"started_at" db:"started_at"`
	ExpiresAt time.Time `json:"expires_at" db:"expires_at"`
}

func (r *Repository) TopicsJSON() string {
	b, _ := json.Marshal(r.Topics)
	return string(b)
}

func (r *Repository) LanguageStatsJSON() string {
	b, _ := json.Marshal(r.LanguageStats)
	return string(b)
}
