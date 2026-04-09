package git

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/errors"
	"github.com/milos85vasic/My-Patreon-Manager/internal/metrics"
)

type TokenManager struct {
	mu             sync.RWMutex
	PrimaryToken   string
	SecondaryToken string
	CurrentToken   string
	OnFailover     func(from, to string)
	cb             *metrics.CircuitBreaker
}

func NewTokenManager(primary, secondary string) *TokenManager {
	return &TokenManager{
		PrimaryToken:   primary,
		SecondaryToken: secondary,
		CurrentToken:   primary,
		cb: metrics.NewCircuitBreaker("token_manager", 3, 60*time.Second, 30*time.Second,
			metrics.DefaultOnTrip, metrics.DefaultOnReset),
	}
}

func (tm *TokenManager) GetCurrentToken() string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.CurrentToken
}

func (tm *TokenManager) Failover() error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.SecondaryToken == "" {
		return errors.RateLimited("no secondary token available", time.Now().Add(1*time.Hour))
	}

	if tm.CurrentToken == tm.PrimaryToken {
		tm.CurrentToken = tm.SecondaryToken
		if tm.OnFailover != nil {
			tm.OnFailover(tm.PrimaryToken, tm.SecondaryToken)
		}
		return nil
	}

	tm.CurrentToken = tm.PrimaryToken
	if tm.OnFailover != nil {
		tm.OnFailover(tm.SecondaryToken, tm.PrimaryToken)
	}
	return nil
}

func (tm *TokenManager) IsFailedOver() bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.CurrentToken != tm.PrimaryToken
}

type RateLimitAwareClient struct {
	client       *http.Client
	tokenManager *TokenManager
	baseURL      string
}

func NewRateLimitAwareClient(tokenManager *TokenManager, baseURL string) *RateLimitAwareClient {
	return &RateLimitAwareClient{
		client:       &http.Client{Timeout: 30 * time.Second},
		tokenManager: tokenManager,
		baseURL:      baseURL,
	}
}

func (c *RateLimitAwareClient) Do(req *http.Request) (*http.Response, error) {
	token := c.tokenManager.GetCurrentToken()
	if token == "" {
		return nil, errors.InvalidCredentials("no token available")
	}
	return c.client.Do(req)
}

func (c *RateLimitAwareClient) HandleRateLimit(resp *http.Response) error {
	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter := resp.Header.Get("Retry-After")
		if retryAfter == "" {
			retryAfter = "3600"
		}
		c.tokenManager.Failover()
		return errors.RateLimited("rate limited", time.Now().Add(1*time.Hour))
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		c.tokenManager.Failover()
		return errors.PermissionDenied("token rejected")
	}
	return nil
}

type OAuth2TokenManager struct {
	mu            sync.RWMutex
	AccessToken   string
	RefreshToken  string
	ClientID      string
	ClientSecret  string
	ExpiresAt     time.Time
	OnTokenUpdate func(accessToken, refreshToken string)
	client        *http.Client
}

func NewOAuth2TokenManager(clientID, clientSecret, accessToken, refreshToken string) *OAuth2TokenManager {
	return &OAuth2TokenManager{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		ExpiresAt:    time.Now().Add(1 * time.Hour),
		client:       &http.Client{Timeout: 30 * time.Second},
	}
}

func (m *OAuth2TokenManager) GetAccessToken() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.AccessToken
}

func (m *OAuth2TokenManager) Refresh(ctx context.Context) error {
	if m.RefreshToken == "" {
		return errors.InvalidCredentials("no refresh token available")
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://www.patreon.com/api/oauth2/token", nil)
	if err != nil {
		return fmt.Errorf("create refresh request: %w", err)
	}
	q := req.URL.Query()
	q.Set("grant_type", "refresh_token")
	q.Set("refresh_token", m.RefreshToken)
	q.Set("client_id", m.ClientID)
	q.Set("client_secret", m.ClientSecret)
	req.URL.RawQuery = q.Encode()

	resp, err := m.client.Do(req)
	if err != nil {
		return errors.NetworkTimeout(fmt.Sprintf("token refresh failed: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.InvalidCredentials(fmt.Sprintf("token refresh returned %d", resp.StatusCode))
	}

	m.mu.Lock()
	m.ExpiresAt = time.Now().Add(1 * time.Hour)
	if m.OnTokenUpdate != nil {
		m.OnTokenUpdate(m.AccessToken, m.RefreshToken)
	}
	m.mu.Unlock()

	return nil
}

func (m *OAuth2TokenManager) IsExpired() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return time.Now().After(m.ExpiresAt)
}
