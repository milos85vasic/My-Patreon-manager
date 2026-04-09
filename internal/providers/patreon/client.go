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
)

type Client struct {
	oauth           *OAuth2Manager
	campaignID      string
	client          *http.Client
	publicationMode string
}

func NewClient(oauth *OAuth2Manager, campaignID string) *Client {
	return &Client{
		oauth:           oauth,
		campaignID:      campaignID,
		client:          &http.Client{Timeout: 30 * time.Second},
		publicationMode: "draft",
	}
}

func (c *Client) SetPublicationMode(mode string) {
	c.publicationMode = mode
}

func (c *Client) doRequest(ctx context.Context, method, url string, body interface{}) (*http.Response, error) {
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

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, errors.NetworkTimeout(fmt.Sprintf("patreon request failed: %v", err))
	}

	if resp.StatusCode == 401 {
		resp.Body.Close()
		if refreshErr := c.oauth.Refresh(ctx); refreshErr != nil {
			return nil, errors.InvalidCredentials("token refresh failed")
		}
		req.Header.Set("Authorization", "Bearer "+c.oauth.GetAccessToken())
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
	url := fmt.Sprintf("https://www.patreon.com/api/oauth2/v2/campaigns/%s?fields[campaign]=name,summary,patron_count", c.campaignID)
	resp, err := c.doWithBackoff(ctx, "GET", url, nil, 3)
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
	url := fmt.Sprintf("https://www.patreon.com/api/oauth2/v2/campaigns/%s/tiers?fields[tier]=title,description,amount_cents,patron_count", c.campaignID)
	resp, err := c.doWithBackoff(ctx, "GET", url, nil, 3)
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
	url := "https://www.patreon.com/api/oauth2/v2/posts"
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

	resp, err := c.doWithBackoff(ctx, "POST", url, body, 3)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode create post: %w", err)
	}
	post.ID = result.Data.ID
	post.PublicationStatus = c.publicationMode
	return post, nil
}

func (c *Client) UpdatePost(ctx context.Context, post *models.Post) (*models.Post, error) {
	url := fmt.Sprintf("https://www.patreon.com/api/oauth2/v2/posts/%s", post.ID)
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

	resp, err := c.doWithBackoff(ctx, "PATCH", url, body, 3)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return post, nil
}

func (c *Client) DeletePost(ctx context.Context, postID string) error {
	url := fmt.Sprintf("https://www.patreon.com/api/oauth2/v2/posts/%s", postID)
	resp, err := c.doWithBackoff(ctx, "DELETE", url, nil, 3)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *Client) AssociateTiers(ctx context.Context, postID string, tierIDs []string) error {
	url := fmt.Sprintf("https://www.patreon.com/api/oauth2/v2/posts/%s", postID)
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

	resp, err := c.doWithBackoff(ctx, "PATCH", url, body, 3)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
