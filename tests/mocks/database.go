package mocks

import (
	"context"
	"database/sql"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

type MockDatabase struct {
	ConnectFunc           func(ctx context.Context, dsn string) error
	CloseFunc             func() error
	MigrateFunc           func(ctx context.Context) error
	RepositoriesFunc      func() database.RepositoryStore
	SyncStatesFunc        func() database.SyncStateStore
	MirrorMapsFunc        func() database.MirrorMapStore
	GeneratedContentsFunc func() database.GeneratedContentStore
	ContentTemplatesFunc  func() database.ContentTemplateStore
	PostsFunc             func() database.PostStore
	AuditEntriesFunc      func() database.AuditEntryStore
	AcquireLockFunc       func(ctx context.Context, lockInfo database.SyncLock) error
	ReleaseLockFunc       func(ctx context.Context) error
	IsLockedFunc          func(ctx context.Context) (bool, *database.SyncLock, error)
	BeginTxFunc           func(ctx context.Context) (*sql.Tx, error)
}

func (m *MockDatabase) Connect(ctx context.Context, dsn string) error {
	if m.ConnectFunc != nil {
		return m.ConnectFunc(ctx, dsn)
	}
	return nil
}

func (m *MockDatabase) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

func (m *MockDatabase) Migrate(ctx context.Context) error {
	if m.MigrateFunc != nil {
		return m.MigrateFunc(ctx)
	}
	return nil
}

func (m *MockDatabase) Repositories() database.RepositoryStore {
	if m.RepositoriesFunc != nil {
		return m.RepositoriesFunc()
	}
	return nil
}

func (m *MockDatabase) SyncStates() database.SyncStateStore {
	if m.SyncStatesFunc != nil {
		return m.SyncStatesFunc()
	}
	return nil
}

func (m *MockDatabase) MirrorMaps() database.MirrorMapStore {
	if m.MirrorMapsFunc != nil {
		return m.MirrorMapsFunc()
	}
	return nil
}

func (m *MockDatabase) GeneratedContents() database.GeneratedContentStore {
	if m.GeneratedContentsFunc != nil {
		return m.GeneratedContentsFunc()
	}
	return nil
}

func (m *MockDatabase) ContentTemplates() database.ContentTemplateStore {
	if m.ContentTemplatesFunc != nil {
		return m.ContentTemplatesFunc()
	}
	return nil
}

func (m *MockDatabase) Posts() database.PostStore {
	if m.PostsFunc != nil {
		return m.PostsFunc()
	}
	return nil
}

func (m *MockDatabase) AuditEntries() database.AuditEntryStore {
	if m.AuditEntriesFunc != nil {
		return m.AuditEntriesFunc()
	}
	return nil
}

func (m *MockDatabase) AcquireLock(ctx context.Context, lockInfo database.SyncLock) error {
	if m.AcquireLockFunc != nil {
		return m.AcquireLockFunc(ctx, lockInfo)
	}
	return nil
}

func (m *MockDatabase) ReleaseLock(ctx context.Context) error {
	if m.ReleaseLockFunc != nil {
		return m.ReleaseLockFunc(ctx)
	}
	return nil
}

func (m *MockDatabase) IsLocked(ctx context.Context) (bool, *database.SyncLock, error) {
	if m.IsLockedFunc != nil {
		return m.IsLockedFunc(ctx)
	}
	return false, nil, nil
}

func (m *MockDatabase) BeginTx(ctx context.Context) (*sql.Tx, error) {
	if m.BeginTxFunc != nil {
		return m.BeginTxFunc(ctx)
	}
	return nil, nil
}

type MockRepositoryStore struct {
	CreateFunc  func(ctx context.Context, repo *models.Repository) error
	GetByIDFunc func(ctx context.Context, id string) (*models.Repository, error)
	ListFunc    func(ctx context.Context, filter database.RepositoryFilter) ([]*models.Repository, error)
	UpdateFunc  func(ctx context.Context, repo *models.Repository) error
	DeleteFunc  func(ctx context.Context, id string) error
}

func (m *MockRepositoryStore) Create(ctx context.Context, repo *models.Repository) error {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, repo)
	}
	return nil
}

func (m *MockRepositoryStore) GetByID(ctx context.Context, id string) (*models.Repository, error) {
	if m.GetByIDFunc != nil {
		return m.GetByIDFunc(ctx, id)
	}
	return nil, nil
}

func (m *MockRepositoryStore) GetByServiceOwnerName(ctx context.Context, service, owner, name string) (*models.Repository, error) {
	return nil, nil
}

func (m *MockRepositoryStore) List(ctx context.Context, filter database.RepositoryFilter) ([]*models.Repository, error) {
	if m.ListFunc != nil {
		return m.ListFunc(ctx, filter)
	}
	return nil, nil
}

func (m *MockRepositoryStore) Update(ctx context.Context, repo *models.Repository) error {
	if m.UpdateFunc != nil {
		return m.UpdateFunc(ctx, repo)
	}
	return nil
}

func (m *MockRepositoryStore) Delete(ctx context.Context, id string) error {
	if m.DeleteFunc != nil {
		return m.DeleteFunc(ctx, id)
	}
	return nil
}
