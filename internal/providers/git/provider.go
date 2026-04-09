package git

import (
	"context"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

type Credentials struct {
	PrimaryToken   string
	SecondaryToken string
}

type ListOptions struct {
	Page    int
	PerPage int
}

type RepositoryProvider interface {
	Name() string
	Authenticate(ctx context.Context, credentials Credentials) error
	ListRepositories(ctx context.Context, org string, opts ListOptions) ([]models.Repository, error)
	GetRepositoryMetadata(ctx context.Context, repo models.Repository) (models.Repository, error)
	DetectMirrors(ctx context.Context, repos []models.Repository) ([]models.MirrorMap, error)
	CheckRepositoryState(ctx context.Context, repo models.Repository) (models.SyncState, error)
}
