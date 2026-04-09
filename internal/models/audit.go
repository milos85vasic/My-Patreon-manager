package models

import (
	"time"
)

type AuditEntry struct {
	ID               string    `json:"id" db:"id"`
	RepositoryID     string    `json:"repository_id" db:"repository_id"`
	EventType        string    `json:"event_type" db:"event_type"`
	SourceState      string    `json:"source_state" db:"source_state"`
	GenerationParams string    `json:"generation_params" db:"generation_params"`
	PublicationMeta  string    `json:"publication_meta" db:"publication_meta"`
	Actor            string    `json:"actor" db:"actor"`
	Outcome          string    `json:"outcome" db:"outcome"`
	ErrorMessage     string    `json:"error_message" db:"error_message"`
	Timestamp        time.Time `json:"timestamp" db:"timestamp"`
}

type SourceState struct {
	Service       string `json:"service"`
	Owner         string `json:"owner"`
	Name          string `json:"name"`
	LastCommitSHA string `json:"last_commit_sha"`
	IsArchived    bool   `json:"is_archived"`
	Description   string `json:"description"`
	Stars         int    `json:"stars"`
}

type GenerationParams struct {
	ModelID        string `json:"model_id"`
	PromptTemplate string `json:"prompt_template"`
	ContentType    string `json:"content_type"`
	TokenCount     int    `json:"token_count"`
	Attempts       int    `json:"attempts"`
}

type PublicationMeta struct {
	PatreonPostID string   `json:"patreon_post_id"`
	TierIDs       []string `json:"tier_ids"`
	Format        string   `json:"format"`
	Status        string   `json:"status"`
}
