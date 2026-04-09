package mocks

import (
	"context"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/git"
)

type MockRepositoryProvider struct {
	NameFunc             func() string
	AuthenticateFunc     func(ctx context.Context, credentials git.Credentials) error
	ListRepositoriesFunc func(ctx context.Context, org string, opts git.ListOptions) ([]models.Repository, error)
	GetMetadataFunc      func(ctx context.Context, repo models.Repository) (models.Repository, error)
	DetectMirrorsFunc    func(ctx context.Context, repos []models.Repository) ([]models.MirrorMap, error)
	CheckStateFunc       func(ctx context.Context, repo models.Repository) (models.SyncState, error)
}

func (m *MockRepositoryProvider) Name() string {
	if m.NameFunc != nil {
		return m.NameFunc()
	}
	return "mock"
}

func (m *MockRepositoryProvider) Authenticate(ctx context.Context, credentials git.Credentials) error {
	if m.AuthenticateFunc != nil {
		return m.AuthenticateFunc(ctx, credentials)
	}
	return nil
}

func (m *MockRepositoryProvider) ListRepositories(ctx context.Context, org string, opts git.ListOptions) ([]models.Repository, error) {
	if m.ListRepositoriesFunc != nil {
		return m.ListRepositoriesFunc(ctx, org, opts)
	}
	return nil, nil
}

func (m *MockRepositoryProvider) GetRepositoryMetadata(ctx context.Context, repo models.Repository) (models.Repository, error) {
	if m.GetMetadataFunc != nil {
		return m.GetMetadataFunc(ctx, repo)
	}
	return models.Repository{}, nil
}

func (m *MockRepositoryProvider) DetectMirrors(ctx context.Context, repos []models.Repository) ([]models.MirrorMap, error) {
	if m.DetectMirrorsFunc != nil {
		return m.DetectMirrorsFunc(ctx, repos)
	}
	return nil, nil
}

func (m *MockRepositoryProvider) CheckRepositoryState(ctx context.Context, repo models.Repository) (models.SyncState, error) {
	if m.CheckStateFunc != nil {
		return m.CheckStateFunc(ctx, repo)
	}
	return models.SyncState{}, nil
}
