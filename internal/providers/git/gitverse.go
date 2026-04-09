package git

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/errors"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/utils"
)

type GitVerseProvider struct {
	client       *http.Client
	tm           *TokenManager
	baseURL      string
	capabilities map[string]bool
}

func NewGitVerseProvider(tokenManager *TokenManager) *GitVerseProvider {
	return &GitVerseProvider{
		client:       &http.Client{Timeout: 30 * time.Second},
		tm:           tokenManager,
		baseURL:      "https://gitverse.ru/api/v1",
		capabilities: make(map[string]bool),
	}
}

func (p *GitVerseProvider) Name() string { return "gitverse" }

func (p *GitVerseProvider) Authenticate(ctx context.Context, credentials Credentials) error {
	if credentials.PrimaryToken == "" {
		return errors.InvalidCredentials("GitVerse token is required")
	}
	p.tm = NewTokenManager(credentials.PrimaryToken, credentials.SecondaryToken)
	p.detectCapabilities(ctx)
	return nil
}

func (p *GitVerseProvider) detectCapabilities(ctx context.Context) {
	endpoints := map[string]string{
		"topics":    fmt.Sprintf("%s/topics", p.baseURL),
		"templates": fmt.Sprintf("%s/templates", p.baseURL),
	}
	for name, url := range endpoints {
		req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
		req.Header.Set("Authorization", "Bearer "+p.tm.GetCurrentToken())
		resp, err := p.client.Do(req)
		if err == nil {
			resp.Body.Close()
			p.capabilities[name] = resp.StatusCode == 200
		}
	}
}

func (p *GitVerseProvider) ListRepositories(ctx context.Context, org string, opts ListOptions) ([]models.Repository, error) {
	perPage := opts.PerPage
	if perPage <= 0 {
		perPage = 100
	}
	page := opts.Page
	if page <= 0 {
		page = 1
	}

	url := fmt.Sprintf("%s/orgs/%s/repos?page=%d&per_page=%d", p.baseURL, org, page, perPage)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("gitverse list repos: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.tm.GetCurrentToken())

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, errors.NetworkTimeout(fmt.Sprintf("gitverse list repos: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode == 403 {
		p.tm.Failover()
		return nil, errors.RateLimited("gitverse rate limited", time.Now().Add(1*time.Hour))
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("gitverse list repos: status %d", resp.StatusCode)
	}

	var gvRepos []struct {
		ID          int      `json:"id"`
		Name        string   `json:"name"`
		FullName    string   `json:"full_name"`
		Description string   `json:"description"`
		HTMLUrl     string   `json:"html_url"`
		SSHUrl      string   `json:"ssh_url"`
		Stars       int      `json:"stargazers_count"`
		Forks       int      `json:"forks_count"`
		Language    string   `json:"language"`
		Topics      []string `json:"topics"`
		Archived    bool     `json:"archived"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&gvRepos); err != nil {
		return nil, fmt.Errorf("gitverse decode: %w", err)
	}

	var allRepos []models.Repository
	for _, r := range gvRepos {
		owner := ""
		if r.FullName != "" {
			parts := splitFullName(r.FullName)
			if len(parts) > 0 {
				owner = parts[0]
			}
		}
		repo := models.Repository{
			ID:              utils.NewUUID(),
			Service:         "gitverse",
			Owner:           owner,
			Name:            r.Name,
			URL:             r.SSHUrl,
			HTTPSURL:        r.HTMLUrl,
			Description:     r.Description,
			PrimaryLanguage: r.Language,
			Stars:           r.Stars,
			Forks:           r.Forks,
			IsArchived:      r.Archived,
			Topics:          r.Topics,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		}
		allRepos = append(allRepos, repo)
	}
	return allRepos, nil
}

func (p *GitVerseProvider) GetRepositoryMetadata(ctx context.Context, repo models.Repository) (models.Repository, error) {
	url := fmt.Sprintf("%s/repos/%s/%s", p.baseURL, repo.Owner, repo.Name)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return repo, fmt.Errorf("gitverse get metadata: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.tm.GetCurrentToken())

	resp, err := p.client.Do(req)
	if err != nil {
		return repo, errors.NetworkTimeout(fmt.Sprintf("gitverse get metadata: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		repo.IsArchived = true
		return repo, nil
	}
	if resp.StatusCode != 200 {
		return repo, fmt.Errorf("gitverse get metadata: status %d", resp.StatusCode)
	}

	var gvRepo struct {
		Description string   `json:"description"`
		Stars       int      `json:"stargazers_count"`
		Forks       int      `json:"forks_count"`
		Language    string   `json:"language"`
		Topics      []string `json:"topics"`
		Archived    bool     `json:"archived"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&gvRepo); err != nil {
		return repo, fmt.Errorf("gitverse decode metadata: %w", err)
	}

	repo.Description = gvRepo.Description
	repo.PrimaryLanguage = gvRepo.Language
	repo.Stars = gvRepo.Stars
	repo.Forks = gvRepo.Forks
	repo.IsArchived = gvRepo.Archived
	if p.capabilities["topics"] {
		repo.Topics = gvRepo.Topics
	}
	return repo, nil
}

func (p *GitVerseProvider) DetectMirrors(ctx context.Context, repos []models.Repository) ([]models.MirrorMap, error) {
	return nil, nil
}

func (p *GitVerseProvider) CheckRepositoryState(ctx context.Context, repo models.Repository) (models.SyncState, error) {
	return models.SyncState{RepositoryID: repo.ID, Status: "active"}, nil
}

func splitFullName(name string) []string {
	var parts []string
	for _, s := range splitString(name, "/") {
		if s != "" {
			parts = append(parts, s)
		}
	}
	return parts
}

func splitString(s, sep string) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if i+len(sep) <= len(s) && s[i:i+len(sep)] == sep {
			result = append(result, s[start:i])
			start = i + len(sep)
			i += len(sep) - 1
		}
	}
	result = append(result, s[start:])
	return result
}
