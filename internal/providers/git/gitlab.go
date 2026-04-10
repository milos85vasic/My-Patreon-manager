package git

import (
	"context"
	"fmt"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/errors"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/utils"
	"github.com/xanzy/go-gitlab"
)

type GitLabProvider struct {
	client  *gitlab.Client
	tm      *TokenManager
	baseURL string
}

func NewGitLabProvider(tokenManager *TokenManager, baseURL string) *GitLabProvider {
	p := &GitLabProvider{tm: tokenManager, baseURL: baseURL}
	// Create client with empty token (will be replaced in Authenticate)
	// If baseURL is provided, use it
	var err error
	if baseURL != "" && baseURL != "https://gitlab.com" {
		p.client, err = gitlab.NewClient("", gitlab.WithBaseURL(baseURL))
	} else {
		p.client, err = gitlab.NewClient("")
	}
	if err != nil {
		// Should not happen with valid URL; fallback to nil client
		p.client = nil
	}
	return p
}

func (p *GitLabProvider) Name() string { return "gitlab" }

func (p *GitLabProvider) Authenticate(ctx context.Context, credentials Credentials) error {
	if credentials.PrimaryToken == "" {
		return errors.InvalidCredentials("GitLab token is required")
	}
	p.tm = NewTokenManager(credentials.PrimaryToken, credentials.SecondaryToken)

	var err error
	if p.baseURL != "" && p.baseURL != "https://gitlab.com" {
		p.client, err = gitlab.NewClient(credentials.PrimaryToken, gitlab.WithBaseURL(p.baseURL))
	} else {
		p.client, err = gitlab.NewClient(credentials.PrimaryToken)
	}
	if err != nil {
		return errors.InvalidCredentials(fmt.Sprintf("GitLab auth failed: %v", err))
	}
	return nil
}

func (p *GitLabProvider) ListRepositories(ctx context.Context, group string, opts ListOptions) ([]models.Repository, error) {
	perPage := opts.PerPage
	if perPage <= 0 {
		perPage = 100
	}
	page := opts.Page
	if page <= 0 {
		page = 1
	}

	var allRepos []models.Repository
	for {
		projects, resp, err := p.client.Groups.ListGroupProjects(group, &gitlab.ListGroupProjectsOptions{
			ListOptions:      gitlab.ListOptions{Page: page, PerPage: perPage},
			IncludeSubGroups: gitlab.Ptr(true),
		})
		if err != nil {
			if resp != nil && resp.StatusCode == 403 {
				p.tm.Failover()
				return nil, errors.RateLimited("GitLab rate limited", time.Now().Add(1*time.Hour))
			}
			return nil, fmt.Errorf("gitlab list repos: %w", err)
		}

		for _, proj := range projects {
			allRepos = append(allRepos, p.projectToRepo(proj))
		}

		if resp.NextPage == 0 {
			break
		}
		page = resp.NextPage
	}
	return allRepos, nil
}

func (p *GitLabProvider) GetRepositoryMetadata(ctx context.Context, repo models.Repository) (models.Repository, error) {
	proj, _, err := p.client.Projects.GetProject(fmt.Sprintf("%s/%s", repo.Owner, repo.Name), &gitlab.GetProjectOptions{
		Statistics: gitlab.Ptr(true),
	})
	if err != nil {
		return repo, fmt.Errorf("gitlab get metadata: %w", err)
	}
	result := p.projectToRepo(proj)
	result.ID = repo.ID

	// fetch latest commit SHA
	commits, _, err := p.client.Commits.ListCommits(fmt.Sprintf("%s/%s", repo.Owner, repo.Name), &gitlab.ListCommitsOptions{
		ListOptions: gitlab.ListOptions{PerPage: 1},
	})
	if err == nil && len(commits) > 0 {
		result.LastCommitSHA = commits[0].ID
	}

	return result, nil
}

func (p *GitLabProvider) DetectMirrors(ctx context.Context, repos []models.Repository) ([]models.MirrorMap, error) {
	return DetectMirrors(ctx, repos)
}

func (p *GitLabProvider) CheckRepositoryState(ctx context.Context, repo models.Repository) (models.SyncState, error) {
	proj, _, err := p.client.Projects.GetProject(fmt.Sprintf("%s/%s", repo.Owner, repo.Name), nil)
	if err != nil {
		return models.SyncState{}, fmt.Errorf("gitlab check state: %w", err)
	}
	state := models.SyncState{
		RepositoryID: repo.ID,
		Status:       "active",
	}
	if proj.MarkedForDeletionAt != nil {
		state.Status = "archived"
	}
	if proj.LastActivityAt != nil {
		state.LastSyncAt = *proj.LastActivityAt
	}
	return state, nil
}

// SetBaseURL sets the base URL for the GitLab client (for testing).
func (p *GitLabProvider) SetBaseURL(baseURL string) error {
	p.baseURL = baseURL
	// Recreate client with new base URL
	var err error
	var token string
	if p.tm != nil {
		token = p.tm.GetCurrentToken()
	} else {
		token = ""
	}
	if baseURL != "" && baseURL != "https://gitlab.com" {
		p.client, err = gitlab.NewClient(token, gitlab.WithBaseURL(baseURL))
	} else {
		p.client, err = gitlab.NewClient(token)
	}
	if err != nil {
		return err
	}
	return nil
}

func (p *GitLabProvider) projectToRepo(proj *gitlab.Project) models.Repository {
	repo := models.Repository{
		ID:              utils.NewUUID(),
		Service:         "gitlab",
		Owner:           proj.Namespace.Path,
		Name:            proj.Path,
		URL:             proj.SSHURLToRepo,
		HTTPSURL:        proj.HTTPURLToRepo,
		Description:     proj.Description,
		PrimaryLanguage: "",
		Stars:           proj.StarCount,
		Forks:           proj.ForksCount,
		IsArchived:      proj.Archived,
		CreatedAt:       *proj.CreatedAt,
		UpdatedAt:       *proj.LastActivityAt,
	}
	topics := make([]string, len(proj.TagList))
	for i, t := range proj.TagList {
		topics[i] = t
	}
	repo.Topics = topics
	if proj.LastActivityAt != nil {
		repo.LastCommitAt = *proj.LastActivityAt
	}
	return repo
}
