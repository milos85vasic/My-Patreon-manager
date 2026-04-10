package patreon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/errors"
)

type OAuth2Manager struct {
	clientID      string
	clientSecret  string
	accessToken   string
	refreshToken  string
	client        *http.Client
	onTokenUpdate func(accessToken, refreshToken string)
	tokenEndpoint string
}

func NewOAuth2Manager(clientID, clientSecret, accessToken, refreshToken string) *OAuth2Manager {
	return &OAuth2Manager{
		clientID:      clientID,
		clientSecret:  clientSecret,
		accessToken:   accessToken,
		refreshToken:  refreshToken,
		client:        &http.Client{Timeout: 30 * time.Second},
		tokenEndpoint: "https://www.patreon.com/api/oauth2/token",
	}
}

func (m *OAuth2Manager) GetAccessToken() string {
	return m.accessToken
}

func (m *OAuth2Manager) Refresh(ctx context.Context) error {
	if m.refreshToken == "" {
		return errors.InvalidCredentials("no refresh token available")
	}

	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", m.refreshToken)
	data.Set("client_id", m.clientID)
	data.Set("client_secret", m.clientSecret)

	req, err := http.NewRequestWithContext(ctx, "POST", m.tokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("create refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := m.client.Do(req)
	if err != nil {
		return errors.NetworkTimeout(fmt.Sprintf("token refresh failed: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.InvalidCredentials(fmt.Sprintf("token refresh returned %d", resp.StatusCode))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
		Scope        string `json:"scope"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return fmt.Errorf("decode token response: %w", err)
	}

	// Update tokens
	m.accessToken = tokenResp.AccessToken
	if tokenResp.RefreshToken != "" {
		m.refreshToken = tokenResp.RefreshToken
	}
	// Optionally store expires_in for proactive refresh

	if m.onTokenUpdate != nil {
		m.onTokenUpdate(m.accessToken, m.refreshToken)
	}
	return nil
}

func (m *OAuth2Manager) ClearCredentials() {
	m.accessToken = ""
	m.refreshToken = ""
	m.clientID = ""
	m.clientSecret = ""
}

func (m *OAuth2Manager) SetTokenUpdateCallback(cb func(accessToken, refreshToken string)) {
	m.onTokenUpdate = cb
}

// SetTokenEndpoint sets the OAuth2 token endpoint URL for testing.
func (m *OAuth2Manager) SetTokenEndpoint(url string) {
	m.tokenEndpoint = url
}

// SetTransport sets the HTTP client's transport for testing.
func (m *OAuth2Manager) SetTransport(transport http.RoundTripper) {
	m.client.Transport = transport
}
