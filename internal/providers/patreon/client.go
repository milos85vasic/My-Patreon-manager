package patreon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/errors"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/sony/gobreaker"
)

type Client struct {
	oauth           *OAuth2Manager
	campaignID      string
	client          *http.Client
	publicationMode string
	baseURL         string
	maxRetries      int
	cb              *gobreaker.CircuitBreaker
}

type Provider interface {
	CampaignID() string
	ListTiers(ctx context.Context) ([]models.Tier, error)
	CreatePost(ctx context.Context, post *models.Post) (*models.Post, error)
	UpdatePost(ctx context.Context, post *models.Post) (*models.Post, error)
	DeletePost(ctx context.Context, postID string) error
}

func NewClient(oauth *OAuth2Manager, campaignID string) *Client {
	c := &Client{
		oauth:           oauth,
		campaignID:      campaignID,
		client:          &http.Client{Timeout: 30 * time.Second},
		publicationMode: "draft",
		baseURL:         "https://www.patreon.com/api/oauth2/v2",
		maxRetries:      3,
	}
	c.cb = gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        "patreon",
		MaxRequests: 1,
		Interval:    60 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 5
		},
	})
	return c
}

// SetMaxRetries overrides the default per-call retry budget used by
// doWithBackoff. Primarily used by tests to make circuit-breaker math
// deterministic.
func (c *Client) SetMaxRetries(n int) {
	if n < 1 {
		n = 1
	}
	c.maxRetries = n
}

// execMutation wraps a mutating API call with the circuit breaker so
// consecutive upstream failures (5xx, rate limits, network errors) short-
// circuit instead of flooding the Patreon API.
func (c *Client) execMutation(fn func() error) error {
	_, err := c.cb.Execute(func() (interface{}, error) {
		return nil, fn()
	})
	return err
}

func (c *Client) SetPublicationMode(mode string) {
	c.publicationMode = mode
}

func (c *Client) SetBaseURL(baseURL string) {
	c.baseURL = baseURL
}

func (c *Client) SetTransport(transport http.RoundTripper) {
	c.client.Transport = transport
}

func (c *Client) CampaignID() string {
	return c.campaignID
}

func (c *Client) buildURL(path string) string {
	return c.baseURL + path
}

func (c *Client) doRequest(ctx context.Context, method, url string, body interface{}) (*http.Response, error) {
	// Helper to create a new request with current token
	newRequest := func() (*http.Request, error) {
		var reqBody io.Reader
		if body != nil {
			data, err := json.Marshal(body)
			if err != nil {
				return nil, fmt.Errorf("marshal request: %w", err)
			}
			reqBody = bytes.NewReader(data)
		}
		req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.oauth.GetAccessToken())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		return req, nil
	}

	req, err := newRequest()
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, errors.NetworkTimeout(fmt.Sprintf("patreon request failed: %v", err))
	}

	if resp.StatusCode == 401 {
		resp.Body.Close()
		if refreshErr := c.oauth.Refresh(ctx); refreshErr != nil {
			return nil, errors.InvalidCredentials("token refresh failed")
		}
		// Create a new request with the refreshed token
		req, err = newRequest()
		if err != nil {
			return nil, err
		}
		resp, err = c.client.Do(req)
		if err != nil {
			return nil, errors.NetworkTimeout(fmt.Sprintf("patreon retry failed: %v", err))
		}
	}

	if resp.StatusCode == 429 {
		resp.Body.Close()
		retryAfter := time.Duration(60+rand.Intn(30)) * time.Second
		return nil, errors.RateLimited("patreon rate limited", time.Now().Add(retryAfter))
	}

	if resp.StatusCode >= 500 {
		resp.Body.Close()
		return nil, errors.NetworkTimeout(fmt.Sprintf("patreon upstream error: %d", resp.StatusCode))
	}

	return resp, nil
}

func (c *Client) doWithBackoff(ctx context.Context, method, url string, body interface{}, maxRetries int) (*http.Response, error) {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		resp, err := c.doRequest(ctx, method, url, body)
		if err != nil {
			if errors.IsRateLimited(err) {
				backoff := time.Duration(math.Pow(2, float64(i))) * time.Second
				jitter := time.Duration(rand.Intn(1000)) * time.Millisecond
				time.Sleep(backoff + jitter)
				lastErr = err
				continue
			}
			return nil, err
		}
		return resp, nil
	}
	return nil, lastErr
}

func (c *Client) GetCampaign(ctx context.Context) (*models.Campaign, error) {
	url := c.buildURL(fmt.Sprintf("/campaigns/%s?fields[campaign]=name,summary,patron_count", c.campaignID))
	resp, err := c.doWithBackoff(ctx, "GET", url, nil, c.maxRetries)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Data struct {
			ID         string `json:"id"`
			Attributes struct {
				Name        string `json:"name"`
				Summary     string `json:"summary"`
				PatronCount int    `json:"patron_count"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode campaign: %w", err)
	}
	return &models.Campaign{
		ID:          result.Data.ID,
		Name:        result.Data.Attributes.Name,
		Summary:     result.Data.Attributes.Summary,
		PatronCount: result.Data.Attributes.PatronCount,
	}, nil
}

func (c *Client) ListTiers(ctx context.Context) ([]models.Tier, error) {
	url := c.buildURL(fmt.Sprintf("/campaigns/%s/tiers?fields[tier]=title,description,amount_cents,patron_count", c.campaignID))
	resp, err := c.doWithBackoff(ctx, "GET", url, nil, c.maxRetries)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Data []struct {
			ID         string `json:"id"`
			Attributes struct {
				Title       string `json:"title"`
				Description string `json:"description"`
				AmountCents int    `json:"amount_cents"`
				PatronCount int    `json:"patron_count"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode tiers: %w", err)
	}

	tiers := make([]models.Tier, len(result.Data))
	for i, t := range result.Data {
		tiers[i] = models.Tier{
			ID:          t.ID,
			CampaignID:  c.campaignID,
			Title:       t.Attributes.Title,
			Description: t.Attributes.Description,
			AmountCents: t.Attributes.AmountCents,
			PatronCount: t.Attributes.PatronCount,
		}
	}
	return tiers, nil
}

func (c *Client) CreatePost(ctx context.Context, post *models.Post) (*models.Post, error) {
	err := c.execMutation(func() error {
		return c.createPostRaw(ctx, post)
	})
	if err != nil {
		return nil, err
	}
	return post, nil
}

func (c *Client) createPostRaw(ctx context.Context, post *models.Post) error {
	url := c.buildURL("/posts")
	body := map[string]interface{}{
		"data": map[string]interface{}{
			"type": "post",
			"attributes": map[string]interface{}{
				"title":            post.Title,
				"content":          post.Content,
				"post_type":        post.PostType,
				"is_paid":          true,
				"publication_type": c.publicationMode,
			},
			"relationships": map[string]interface{}{
				"campaign": map[string]interface{}{
					"data": map[string]interface{}{"type": "campaign", "id": c.campaignID},
				},
			},
		},
	}

	resp, err := c.doWithBackoff(ctx, "POST", url, body, c.maxRetries)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode create post: %w", err)
	}
	post.ID = result.Data.ID
	post.PublicationStatus = c.publicationMode
	return nil
}

func (c *Client) UpdatePost(ctx context.Context, post *models.Post) (*models.Post, error) {
	err := c.execMutation(func() error {
		return c.updatePostRaw(ctx, post)
	})
	if err != nil {
		return nil, err
	}
	return post, nil
}

func (c *Client) updatePostRaw(ctx context.Context, post *models.Post) error {
	url := c.buildURL(fmt.Sprintf("/posts/%s", post.ID))
	body := map[string]interface{}{
		"data": map[string]interface{}{
			"type": "post",
			"id":   post.ID,
			"attributes": map[string]interface{}{
				"title":   post.Title,
				"content": post.Content,
			},
		},
	}

	resp, err := c.doWithBackoff(ctx, "PATCH", url, body, c.maxRetries)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (c *Client) DeletePost(ctx context.Context, postID string) error {
	return c.execMutation(func() error {
		return c.deletePostRaw(ctx, postID)
	})
}

func (c *Client) deletePostRaw(ctx context.Context, postID string) error {
	url := c.buildURL(fmt.Sprintf("/posts/%s", postID))
	resp, err := c.doWithBackoff(ctx, "DELETE", url, nil, c.maxRetries)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// postFieldsQuery is the common ?include/&fields[post] query fragment
// used by GetPost and ListCampaignPosts. Keeping one definition
// guarantees the two endpoints return the same field set, which
// matters for importer/publisher callers that round-trip between
// them.
const postFieldsQuery = "fields[post]=title,content,url,published_at"

// GetPost fetches a single Patreon post by ID. On HTTP 404 it returns
// (nil, nil) so callers can treat "post no longer exists" as a
// non-error state — this mirrors the store-layer "not found" idiom
// used elsewhere in the codebase. All other non-2xx statuses and
// network errors propagate. Malformed JSON propagates as a decode
// error.
func (c *Client) GetPost(ctx context.Context, postID string) (*models.Post, error) {
	url := c.buildURL(fmt.Sprintf("/posts/%s?%s", postID, postFieldsQuery))
	resp, err := c.doWithBackoff(ctx, "GET", url, nil, c.maxRetries)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errors.NewProviderError(
			fmt.Sprintf("patreon get post: unexpected status %d", resp.StatusCode),
			"unexpected_status", false, time.Time{},
		)
	}

	var result struct {
		Data postData `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode post: %w", err)
	}
	return result.Data.toModel(), nil
}

// postData is the JSON:API `data` envelope for a single post record.
// Shared by GetPost and ListCampaignPosts (which nests a slice of
// these under `data`).
type postData struct {
	ID         string `json:"id"`
	Attributes struct {
		Title       string `json:"title"`
		Content     string `json:"content"`
		URL         string `json:"url"`
		PublishedAt string `json:"published_at"`
	} `json:"attributes"`
}

// toModel converts the JSON:API post payload into the internal
// models.Post, preserving the canonical Patreon post URL so
// downstream consumers (first-run importer, drift detection,
// unmatched-post bookkeeping) can reference the post without a
// second fetch.
func (d postData) toModel() *models.Post {
	p := &models.Post{
		ID:      d.ID,
		Title:   d.Attributes.Title,
		Content: d.Attributes.Content,
		URL:     d.Attributes.URL,
	}
	if d.Attributes.PublishedAt != "" {
		if t, err := time.Parse(time.RFC3339, d.Attributes.PublishedAt); err == nil {
			p.PublishedAt = t
		}
	}
	return p
}

// ListCampaignPosts enumerates every post belonging to the given
// campaign. It follows JSON:API `links.next` pagination until the
// API stops providing a next link, then returns the merged slice.
// An empty campaign yields an empty (non-nil) slice.
func (c *Client) ListCampaignPosts(ctx context.Context, campaignID string) ([]models.Post, error) {
	posts := make([]models.Post, 0)
	url := c.buildURL(fmt.Sprintf("/campaigns/%s/posts?%s&page[size]=20", campaignID, postFieldsQuery))
	for url != "" {
		page, next, err := c.listCampaignPostsPage(ctx, url)
		if err != nil {
			return nil, err
		}
		posts = append(posts, page...)
		url = next
	}
	return posts, nil
}

// listCampaignPostsPage fetches a single page of campaign posts and
// returns the decoded slice plus the absolute URL of the next page
// (empty when this is the last page).
func (c *Client) listCampaignPostsPage(ctx context.Context, url string) ([]models.Post, string, error) {
	resp, err := c.doWithBackoff(ctx, "GET", url, nil, c.maxRetries)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", errors.NewProviderError(
			fmt.Sprintf("patreon list campaign posts: unexpected status %d", resp.StatusCode),
			"unexpected_status", false, time.Time{},
		)
	}

	var result struct {
		Data  []postData `json:"data"`
		Links struct {
			Next string `json:"next"`
		} `json:"links"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, "", fmt.Errorf("decode campaign posts: %w", err)
	}
	out := make([]models.Post, len(result.Data))
	for i, d := range result.Data {
		out[i] = *d.toModel()
	}
	return out, result.Links.Next, nil
}

func (c *Client) AssociateTiers(ctx context.Context, postID string, tierIDs []string) error {
	return c.execMutation(func() error {
		return c.associateTiersRaw(ctx, postID, tierIDs)
	})
}

func (c *Client) associateTiersRaw(ctx context.Context, postID string, tierIDs []string) error {
	url := c.buildURL(fmt.Sprintf("/posts/%s", postID))
	tierData := make([]map[string]interface{}, len(tierIDs))
	for i, id := range tierIDs {
		tierData[i] = map[string]interface{}{"type": "tier", "id": id}
	}
	body := map[string]interface{}{
		"data": map[string]interface{}{
			"type": "post",
			"id":   postID,
			"relationships": map[string]interface{}{
				"tiers": map[string]interface{}{"data": tierData},
			},
		},
	}

	resp, err := c.doWithBackoff(ctx, "PATCH", url, body, c.maxRetries)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
