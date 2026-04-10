package database

import (
	"context"
	"database/sql"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

type SyncLock struct {
	ID        string    `json:"id" db:"id"`
	PID       int       `json:"pid" db:"pid"`
	Hostname  string    `json:"hostname" db:"hostname"`
	StartedAt time.Time `json:"started_at" db:"started_at"`
	ExpiresAt time.Time `json:"expires_at" db:"expires_at"`
}

type Database interface {
	Connect(ctx context.Context, dsn string) error
	Close() error
	Migrate(ctx context.Context) error

	Repositories() RepositoryStore
	SyncStates() SyncStateStore
	MirrorMaps() MirrorMapStore
	GeneratedContents() GeneratedContentStore
	ContentTemplates() ContentTemplateStore
	Posts() PostStore
	AuditEntries() AuditEntryStore

	AcquireLock(ctx context.Context, lockInfo SyncLock) error
	ReleaseLock(ctx context.Context) error
	IsLocked(ctx context.Context) (bool, *SyncLock, error)

	BeginTx(ctx context.Context) (*sql.Tx, error)
}

type RepositoryStore interface {
	Create(ctx context.Context, repo *models.Repository) error
	GetByID(ctx context.Context, id string) (*models.Repository, error)
	GetByServiceOwnerName(ctx context.Context, service, owner, name string) (*models.Repository, error)
	List(ctx context.Context, filter RepositoryFilter) ([]*models.Repository, error)
	Update(ctx context.Context, repo *models.Repository) error
	Delete(ctx context.Context, id string) error
}

type RepositoryFilter struct {
	Service    string
	Owner      string
	IsArchived *bool
}

type SyncStateStore interface {
	Create(ctx context.Context, state *models.SyncState) error
	GetByID(ctx context.Context, id string) (*models.SyncState, error)
	GetByRepositoryID(ctx context.Context, repoID string) (*models.SyncState, error)
	GetByStatus(ctx context.Context, status string) ([]*models.SyncState, error)
	UpdateStatus(ctx context.Context, repoID, status, reason string) error
	UpdateCheckpoint(ctx context.Context, repoID, checkpoint string) error
	Update(ctx context.Context, state *models.SyncState) error
	Delete(ctx context.Context, id string) error
}

type MirrorMapStore interface {
	Create(ctx context.Context, m *models.MirrorMap) error
	GetByMirrorGroupID(ctx context.Context, groupID string) ([]*models.MirrorMap, error)
	GetByRepositoryID(ctx context.Context, repoID string) ([]*models.MirrorMap, error)
	GetAllGroups(ctx context.Context) ([]string, error)
	SetCanonical(ctx context.Context, repoID string) error
	Delete(ctx context.Context, id string) error
	DeleteAll(ctx context.Context) error
}

type GeneratedContentStore interface {
	Create(ctx context.Context, c *models.GeneratedContent) error
	GetByID(ctx context.Context, id string) (*models.GeneratedContent, error)
	GetLatestByRepo(ctx context.Context, repoID string) (*models.GeneratedContent, error)
	GetByQualityRange(ctx context.Context, min, max float64) ([]*models.GeneratedContent, error)
	ListByRepository(ctx context.Context, repoID string) ([]*models.GeneratedContent, error)
	Update(ctx context.Context, c *models.GeneratedContent) error
}

type ContentTemplateStore interface {
	Create(ctx context.Context, t *models.ContentTemplate) error
	GetByName(ctx context.Context, name string) (*models.ContentTemplate, error)
	ListByContentType(ctx context.Context, contentType string) ([]*models.ContentTemplate, error)
	Update(ctx context.Context, t *models.ContentTemplate) error
	Delete(ctx context.Context, id string) error
}

type PostStore interface {
	Create(ctx context.Context, p *models.Post) error
	GetByID(ctx context.Context, id string) (*models.Post, error)
	GetByRepositoryID(ctx context.Context, repoID string) (*models.Post, error)
	Update(ctx context.Context, p *models.Post) error
	UpdatePublicationStatus(ctx context.Context, id, status string) error
	MarkManuallyEdited(ctx context.Context, id string) error
	ListByStatus(ctx context.Context, status string) ([]*models.Post, error)
	Delete(ctx context.Context, id string) error
}

type AuditEntryStore interface {
	Create(ctx context.Context, e *models.AuditEntry) error
	ListByRepository(ctx context.Context, repoID string) ([]*models.AuditEntry, error)
	ListByEventType(ctx context.Context, eventType string) ([]*models.AuditEntry, error)
	ListByTimeRange(ctx context.Context, from, to string) ([]*models.AuditEntry, error)
	PurgeOlderThan(ctx context.Context, cutoff string) (int64, error)
}
