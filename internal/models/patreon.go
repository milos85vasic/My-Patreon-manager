package models

import (
	"encoding/json"
	"time"
)

type Campaign struct {
	ID          string    `json:"id" db:"id"`
	Name        string    `json:"name" db:"name"`
	Summary     string    `json:"summary" db:"summary"`
	CreatorName string    `json:"creator_name" db:"creator_name"`
	PatronCount int       `json:"patron_count" db:"patron_count"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

type Post struct {
	ID                string    `json:"id" db:"id"`
	CampaignID        string    `json:"campaign_id" db:"campaign_id"`
	RepositoryID      string    `json:"repository_id" db:"repository_id"`
	Title             string    `json:"title" db:"title"`
	Content           string    `json:"content" db:"content"`
	URL               string    `json:"url" db:"url"`
	PostType          string    `json:"post_type" db:"post_type"`
	TierIDs           []string  `json:"tier_ids" db:"tier_ids"`
	PublicationStatus string    `json:"publication_status" db:"publication_status"`
	PublishedAt       time.Time `json:"published_at" db:"published_at"`
	IsManuallyEdited  bool      `json:"is_manually_edited" db:"is_manually_edited"`
	ContentHash       string    `json:"content_hash" db:"content_hash"`
	CreatedAt         time.Time `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time `json:"updated_at" db:"updated_at"`
}

type Tier struct {
	ID          string    `json:"id" db:"id"`
	CampaignID  string    `json:"campaign_id" db:"campaign_id"`
	Title       string    `json:"title" db:"title"`
	Description string    `json:"description" db:"description"`
	AmountCents int       `json:"amount_cents" db:"amount_cents"`
	PatronCount int       `json:"patron_count" db:"patron_count"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

func (p *Post) TierIDsJSON() string {
	b, _ := json.Marshal(p.TierIDs)
	return string(b)
}
