package git

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/google/go-github/v69/github"
	"github.com/milos85vasic/My-Patreon-Manager/internal/errors"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/utils"
)

type GitHubProvider struct {
	client *github.Client
	tm     *TokenManager
	org    string
}

func NewGitHubProvider(tokenManager *TokenManager) *GitHubProvider {
	client := github.NewClient(nil)
	return &GitHubProvider{client: client, tm: tokenManager}
}

func (p *GitHubProvider) Name() string { return "github" }

func (p *GitHubProvider) Authenticate(ctx context.Context, credentials Credentials) error {
	if credentials.PrimaryToken == "" {
		return errors.InvalidCredentials("GitHub token is required")
	}
	p.tm = NewTokenManager(credentials.PrimaryToken, credentials.SecondaryToken)
	// Preserve existing base URL if any
	var existingBaseURL *url.URL
	if p.client != nil {
		existingBaseURL = p.client.BaseURL
	}
	p.client = github.NewTokenClient(ctx, credentials.PrimaryToken)
	if existingBaseURL != nil {
		p.client.BaseURL = existingBaseURL
	}
	_, _, err := p.client.Repositories.List(ctx, "", &github.RepositoryListOptions{ListOptions: github.ListOptions{PerPage: 1}})
	if err != nil {
		return errors.InvalidCredentials(fmt.Sprintf("GitHub auth failed: %v", err))
	}
	return nil
}

func (p *GitHubProvider) ListRepositories(ctx context.Context, org string, opts ListOptions) ([]models.Repository, error) {
	if org == "" {
		org = p.org
	}
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
		ghRepos, resp, err := p.client.Repositories.ListByOrg(ctx, org, &github.RepositoryListByOrgOptions{
			ListOptions: github.ListOptions{Page: page, PerPage: perPage},
		})
		if err != nil {
			if resp != nil && resp.StatusCode == 403 {
				p.tm.Failover()
				return nil, errors.RateLimited("GitHub rate limited", time.Now().Add(1*time.Hour))
			}
			return nil, fmt.Errorf("github list repos: %w", err)
		}
		for _, r := range ghRepos {
			if r == nil {
				continue
			}
			allRepos = append(allRepos, p.toRepository(r))
		}
		if resp.NextPage == 0 {
			break
		}
		page = resp.NextPage
	}
	return allRepos, nil
}

func (p *GitHubProvider) GetRepositoryMetadata(ctx context.Context, repo models.Repository) (models.Repository, error) {
	ghRepo, _, err := p.client.Repositories.Get(ctx, repo.Owner, repo.Name)
	if err != nil {
		return repo, fmt.Errorf("github get metadata: %w", err)
	}
	result := p.toRepository(ghRepo)
	result.ID = repo.ID

	readme, _, err := p.client.Repositories.GetReadme(ctx, repo.Owner, repo.Name, nil)
	if err == nil && readme != nil {
		content, decodeErr := readme.GetContent()
		if decodeErr == nil {
			result.READMEContent = content
		}
		result.READMEFormat = "markdown"
	}

	// fetch latest commit SHA
	commits, _, err := p.client.Repositories.ListCommits(ctx, repo.Owner, repo.Name, &github.CommitsListOptions{ListOptions: github.ListOptions{PerPage: 1}})
	if err == nil && len(commits) > 0 {
		result.LastCommitSHA = commits[0].GetSHA()
	}

	return result, nil
}

func (p *GitHubProvider) DetectMirrors(ctx context.Context, repos []models.Repository) ([]models.MirrorMap, error) {
	return DetectMirrors(ctx, repos)
}

func (p *GitHubProvider) CheckRepositoryState(ctx context.Context, repo models.Repository) (models.SyncState, error) {
	ghRepo, _, err := p.client.Repositories.Get(ctx, repo.Owner, repo.Name)
	if err != nil {
		return models.SyncState{}, fmt.Errorf("github check state: %w", err)
	}
	state := models.SyncState{
		RepositoryID: repo.ID,
		Status:       "active",
	}
	if ghRepo.GetArchived() {
		state.Status = "archived"
	}
	if ghRepo.PushedAt != nil {
		state.LastSyncAt = ghRepo.PushedAt.Time
	}
	return state, nil
}

func (p *GitHubProvider) toRepository(r *github.Repository) models.Repository {
	repo := models.Repository{
		ID:              utils.NewUUID(),
		Service:         "github",
		Owner:           r.GetOwner().GetLogin(),
		Name:            r.GetName(),
		URL:             fmt.Sprintf("git@github.com:%s/%s.git", r.GetOwner().GetLogin(), r.GetName()),
		HTTPSURL:        r.GetHTMLURL(),
		Description:     r.GetDescription(),
		PrimaryLanguage: r.GetLanguage(),
		Stars:           r.GetStargazersCount(),
		Forks:           r.GetForksCount(),
		IsArchived:      r.GetArchived(),
		Topics:          r.Topics,
		CreatedAt:       r.GetCreatedAt().Time,
		UpdatedAt:       r.GetUpdatedAt().Time,
	}
	if r.PushedAt != nil {
		repo.LastCommitAt = r.PushedAt.Time
	}
	return repo
}

// SetBaseURL sets the base URL for the GitHub client (for testing).
func (p *GitHubProvider) SetBaseURL(baseURL string) error {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return err
	}
	if p.client != nil {
		p.client.BaseURL = parsed
	}
	return nil
}
