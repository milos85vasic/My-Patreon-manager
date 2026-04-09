package patreon

import (
	"context"
	"fmt"
	"net/http"
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
}

func NewOAuth2Manager(clientID, clientSecret, accessToken, refreshToken string) *OAuth2Manager {
	return &OAuth2Manager{
		clientID:     clientID,
		clientSecret: clientSecret,
		accessToken:  accessToken,
		refreshToken: refreshToken,
		client:       &http.Client{Timeout: 30 * time.Second},
	}
}

func (m *OAuth2Manager) GetAccessToken() string {
	return m.accessToken
}

func (m *OAuth2Manager) Refresh(ctx context.Context) error {
	if m.refreshToken == "" {
		return errors.InvalidCredentials("no refresh token available")
	}

	url := fmt.Sprintf("https://www.patreon.com/api/oauth2/token?grant_type=refresh_token&refresh_token=***&client_id=%s&client_secret=***
		m.refreshToken, m.clientID, m.clientSecret)

	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return fmt.Errorf("create refresh request: %w", err)
	}

	resp, err := m.client.Do(req)
	if err != nil {
		return errors.NetworkTimeout(fmt.Sprintf("token refresh failed: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.InvalidCredentials(fmt.Sprintf("token refresh returned %d", resp.StatusCode))
	}

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
