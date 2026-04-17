package git

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/errors"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/utils"
)

type GitFlicProvider struct {
	client  *http.Client
	tm      *TokenManager
	baseURL string
}

func NewGitFlicProvider(tokenManager *TokenManager) *GitFlicProvider {
	return &GitFlicProvider{
		client:  &http.Client{Timeout: 30 * time.Second},
		tm:      tokenManager,
		baseURL: "https://gitflic.ru/api/v1",
	}
}

func (p *GitFlicProvider) Name() string { return "gitflic" }

// doRequest sends req through the HTTP client while routing the call
// through the TokenManager circuit breaker. Network errors and 5xx
// responses count as upstream failures and can trip the breaker; 4xx
// responses are returned to the caller without tripping it.
func (p *GitFlicProvider) doRequest(req *http.Request) (*http.Response, error) {
	var (
		resp   *http.Response
		cliErr error
	)
	_, bErr := p.tm.Execute(func() (interface{}, error) {
		r, err := p.client.Do(req)
		if err != nil {
			cliErr = err
			return nil, err
		}
		if r.StatusCode >= 500 {
			body := r
			// Drain/close the body so the upstream caller can't leak
			// a connection. We still return the status via a synthesized
			// response so callers can inspect StatusCode if they wish.
			body.Body.Close()
			resp = &http.Response{StatusCode: r.StatusCode, Header: r.Header}
			return nil, fmt.Errorf("gitflic upstream %d", r.StatusCode)
		}
		resp = r
		return r, nil
	})
	if bErr != nil {
		if cliErr != nil {
			return nil, cliErr
		}
		if resp != nil {
			return resp, bErr
		}
		return nil, bErr
	}
	return resp, nil
}

func (p *GitFlicProvider) SetBaseURL(baseURL string) error {
	p.baseURL = baseURL
	return nil
}

func (p *GitFlicProvider) Authenticate(ctx context.Context, credentials Credentials) error {
	if credentials.PrimaryToken == "" {
		return errors.InvalidCredentials("GitFlic token is required")
	}
	p.tm = NewTokenManager(credentials.PrimaryToken, credentials.SecondaryToken)
	req, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/user", nil)
	if err != nil {
		return fmt.Errorf("gitflic auth: %w", err)
	}
	req.Header.Set("Authorization", "token "+credentials.PrimaryToken)
	resp, err := p.doRequest(req)
	if err != nil {
		return errors.NetworkTimeout(fmt.Sprintf("gitflic auth: %v", err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return errors.InvalidCredentials(fmt.Sprintf("gitflic auth failed: %d", resp.StatusCode))
	}
	return nil
}

func (p *GitFlicProvider) ListRepositories(ctx context.Context, org string, opts ListOptions) ([]models.Repository, error) {
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
		var apiURL string
		if org == "" {
			apiURL = fmt.Sprintf("%s/user/repos?page=%d&per_page=%d", p.baseURL, page, perPage)
		} else {
			apiURL = fmt.Sprintf("%s/orgs/%s/repos?page=%d&per_page=%d", p.baseURL, org, page, perPage)
		}
		url := apiURL
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("gitflic list repos: %w", err)
		}
		req.Header.Set("Authorization", "token "+p.tm.GetCurrentToken())

		resp, err := p.doRequest(req)
		if err != nil {
			return nil, errors.NetworkTimeout(fmt.Sprintf("gitflic list repos: %v", err))
		}
		defer resp.Body.Close()

		if resp.StatusCode == 403 {
			p.tm.Failover()
			return nil, errors.RateLimited("gitflic rate limited", time.Now().Add(1*time.Hour))
		}
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("gitflic list repos: status %d", resp.StatusCode)
		}

		var gfRepos []struct {
			ID          int    `json:"id"`
			Title       string `json:"title"`
			Alias       string `json:"alias"`
			Description string `json:"description"`
			Owner       string `json:"owner"`
			OwnerAlias  string `json:"ownerAlias"`
			HTTPUrl     string `json:"httpUrl"`
			SSHUrl      string `json:"sshUrl"`
			Stars       int    `json:"stars"`
			Forks       int    `json:"forks"`
			Language    string `json:"language"`
			IsPrivate   bool   `json:"isPrivate"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&gfRepos); err != nil {
			return nil, fmt.Errorf("gitflic decode: %w", err)
		}

		for _, r := range gfRepos {
			repo := models.Repository{
				ID:              utils.NewUUID(),
				Service:         "gitflic",
				Owner:           r.OwnerAlias,
				Name:            r.Alias,
				URL:             r.SSHUrl,
				HTTPSURL:        r.HTTPUrl,
				Description:     r.Description,
				PrimaryLanguage: r.Language,
				Stars:           r.Stars,
				Forks:           r.Forks,
				IsArchived:      false,
				CreatedAt:       time.Now(),
				UpdatedAt:       time.Now(),
			}
			allRepos = append(allRepos, repo)
		}

		totalStr := resp.Header.Get("X-Total-Pages")
		if totalStr == "" {
			break
		}
		totalPages, _ := strconv.Atoi(totalStr)
		if page >= totalPages {
			break
		}
		page++
	}
	return allRepos, nil
}

func (p *GitFlicProvider) GetRepositoryMetadata(ctx context.Context, repo models.Repository) (models.Repository, error) {
	url := fmt.Sprintf("%s/repos/%s/%s", p.baseURL, repo.Owner, repo.Name)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return repo, fmt.Errorf("gitflic get metadata: %w", err)
	}
	req.Header.Set("Authorization", "token "+p.tm.GetCurrentToken())

	resp, err := p.doRequest(req)
	if err != nil {
		return repo, errors.NetworkTimeout(fmt.Sprintf("gitflic get metadata: %v", err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return repo, fmt.Errorf("gitflic get metadata: status %d", resp.StatusCode)
	}

	var gfRepo struct {
		Title       string `json:"title"`
		Alias       string `json:"alias"`
		Description string `json:"description"`
		HTTPUrl     string `json:"httpUrl"`
		SSHUrl      string `json:"sshUrl"`
		Stars       int    `json:"stars"`
		Forks       int    `json:"forks"`
		Language    string `json:"language"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&gfRepo); err != nil {
		return repo, fmt.Errorf("gitflic decode metadata: %w", err)
	}

	repo.Description = gfRepo.Description
	repo.PrimaryLanguage = gfRepo.Language
	repo.Stars = gfRepo.Stars
	repo.Forks = gfRepo.Forks

	// fetch latest commit SHA
	commitsURL := fmt.Sprintf("%s/repos/%s/%s/commits?per_page=1", p.baseURL, repo.Owner, repo.Name)
	req2, err := http.NewRequestWithContext(ctx, "GET", commitsURL, nil)
	if err == nil {
		req2.Header.Set("Authorization", "token "+p.tm.GetCurrentToken())
		resp2, err := p.doRequest(req2)
		if err == nil {
			defer resp2.Body.Close()
			if resp2.StatusCode == 200 {
				var commits []struct {
					Sha string `json:"sha"`
				}
				if json.NewDecoder(resp2.Body).Decode(&commits) == nil && len(commits) > 0 {
					repo.LastCommitSHA = commits[0].Sha
				}
			}
		}
	}

	return repo, nil
}

func (p *GitFlicProvider) DetectMirrors(ctx context.Context, repos []models.Repository) ([]models.MirrorMap, error) {
	return DetectMirrors(ctx, repos)
}

func (p *GitFlicProvider) CheckRepositoryState(ctx context.Context, repo models.Repository) (models.SyncState, error) {
	return models.SyncState{RepositoryID: repo.ID, Status: "active"}, nil
}
