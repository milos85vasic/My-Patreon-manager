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
	Illustrations() IllustrationStore
	ContentRevisions() ContentRevisionStore
	ProcessRuns() ProcessRunStore
	UnmatchedPatreonPosts() UnmatchedPatreonPostStore

	AcquireLock(ctx context.Context, lockInfo SyncLock) error
	ReleaseLock(ctx context.Context) error
	IsLocked(ctx context.Context) (bool, *SyncLock, error)

	BeginTx(ctx context.Context) (*sql.Tx, error)

	// Dialect identifies the underlying SQL dialect so callers that need
	// to build raw SQL outside the store layer (e.g. the process-command
	// pipeline's per-run transaction) can pick the right placeholder
	// syntax. Returns "sqlite" or "postgres".
	Dialect() string
}

type RepositoryStore interface {
	Create(ctx context.Context, repo *models.Repository) error
	GetByID(ctx context.Context, id string) (*models.Repository, error)
	GetByServiceOwnerName(ctx context.Context, service, owner, name string) (*models.Repository, error)
	List(ctx context.Context, filter RepositoryFilter) ([]*models.Repository, error)
	Update(ctx context.Context, repo *models.Repository) error
	Delete(ctx context.Context, id string) error
	// SetRevisionPointers updates current_revision_id and (optionally)
	// published_revision_id for a repository. If publishedID is empty,
	// published_revision_id is left unchanged; otherwise it is overwritten.
	SetRevisionPointers(ctx context.Context, repoID, currentID, publishedID string) error
	// SetProcessState overwrites the process_state column.
	SetProcessState(ctx context.Context, repoID, state string) error
	// SetLastProcessedAt overwrites the last_processed_at column.
	SetLastProcessedAt(ctx context.Context, repoID string, t time.Time) error
	// ListForProcessQueue returns every repository row in fair-queue
	// order: least-recently-processed first (NULL last_processed_at ranks
	// before any timestamp), id ASC as a stable tiebreaker. No filtering
	// (archived, up-to-date, pending cap) is applied here — the
	// process-command queue builder is responsible for those checks.
	ListForProcessQueue(ctx context.Context) ([]*models.Repository, error)
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

type IllustrationStore interface {
	Create(ctx context.Context, ill *models.Illustration) error
	GetByID(ctx context.Context, id string) (*models.Illustration, error)
	GetByContentID(ctx context.Context, contentID string) (*models.Illustration, error)
	GetByFingerprint(ctx context.Context, fingerprint string) (*models.Illustration, error)
	ListByRepository(ctx context.Context, repoID string) ([]*models.Illustration, error)
	Delete(ctx context.Context, id string) error
}
