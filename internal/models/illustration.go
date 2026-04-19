package models

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

type Illustration struct {
	ID                 string    `json:"id" db:"id"`
	GeneratedContentID string    `json:"generated_content_id" db:"generated_content_id"`
	RepositoryID       string    `json:"repository_id" db:"repository_id"`
	FilePath           string    `json:"file_path" db:"file_path"`
	ImageURL           string    `json:"image_url" db:"image_url"`
	Prompt             string    `json:"prompt" db:"prompt"`
	Style              string    `json:"style" db:"style"`
	ProviderUsed       string    `json:"provider_used" db:"provider_used"`
	Format             string    `json:"format" db:"format"`
	Size               string    `json:"size" db:"size"`
	ContentHash        string    `json:"content_hash" db:"content_hash"`
	Fingerprint        string    `json:"fingerprint" db:"fingerprint"`
	CreatedAt          time.Time `json:"created_at" db:"created_at"`
}

func (i *Illustration) GenerateID() string {
	// Include the fingerprint (prompt+style digest) so illustrations for the
	// same (content, repo) pair but different prompts/styles don't collide
	// on ID. Also important for the revision-based pipeline where
	// GeneratedContentID is empty and many illustrations share a
	// RepositoryID — without the fingerprint those would all hash to the
	// same ID and collide on the primary key.
	h := sha256.Sum256([]byte(i.GeneratedContentID + "|" + i.RepositoryID + "|" + i.Fingerprint))
	i.ID = "ill_" + hex.EncodeToString(h[:])[:32]
	return i.ID
}

func (i *Illustration) ComputeFingerprint() string {
	h := sha256.Sum256([]byte(i.Prompt + i.Style))
	i.Fingerprint = hex.EncodeToString(h[:])
	return i.Fingerprint
}

func (i *Illustration) SetDefaults() {
	if i.Format == "" {
		i.Format = "png"
	}
	if i.Size == "" {
		i.Size = "1792x1024"
	}
	if i.ID == "" {
		i.GenerateID()
	}
	if i.CreatedAt.IsZero() {
		i.CreatedAt = time.Now()
	}
}
