

package patreon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/errors"
	"github.com/stretchr/testify/assert"
)

// newTestOAuth2Manager creates an OAuth2Manager configured to use the test server's token endpoint.
func newTestOAuth2Manager(t *testing.T, serverURL string) *OAuth2Manager {
	t.Helper()
	manager := NewOAuth2Manager("test-client-id", "test-client-secret", "initial-access", "initial-refresh")
	// Set token endpoint to the test server URL (including path)
	manager.SetTokenEndpoint(serverURL + "/api/oauth2/token")
	return manager
}

// errorTransport always returns an error.
type errorTransport struct{}

func (e *errorTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("simulated network error")
}

// isNetworkTimeout checks if an error is a ProviderError with code "network_timeout".
func isNetworkTimeout(err error) bool {
	if pe, ok := err.(errors.ProviderError); ok {
		return pe.Code() == "network_timeout"
	}
	return false
}

func TestOAuth2Manager_GetAccessToken(t *testing.T) {
	manager := NewOAuth2Manager("cid", "secret", "my-token", "refresh")
	assert.Equal(t, "my-token", manager.GetAccessToken())
}

func TestOAuth2Manager_ClearCredentials(t *testing.T) {
	manager := NewOAuth2Manager("cid", "secret", "access", "refresh")
	manager.ClearCredentials()
	// After clearing, GetAccessToken should return empty string
	assert.Equal(t, "", manager.GetAccessToken())
	// Refresh token should also be empty; we cannot directly check but we can attempt refresh
	// which will return InvalidCredentials.
	err := manager.Refresh(context.Background())
	assert.Error(t, err)
	assert.True(t, errors.IsInvalidCredentials(err))
}

func TestOAuth2Manager_Refresh_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/oauth2/token", r.URL.Path)
		assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))
		err := r.ParseForm()
		assert.NoError(t, err)
		assert.Equal(t, "refresh_token", r.Form.Get("grant_type"))
		assert.Equal(t, "initial-refresh", r.Form.Get("refresh_token"))
		assert.Equal(t, "test-client-id", r.Form.Get("client_id"))
		assert.Equal(t, "test-client-secret", r.Form.Get("client_secret"))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "new-access-token",
			"refresh_token": "new-refresh-token",
			"expires_in":    3600,
			"token_type":    "Bearer",
			"scope":         "campaigns posts",
		})
	}))
	defer server.Close()

	manager := newTestOAuth2Manager(t, server.URL)

	// Set a callback to capture token updates
	var updatedAccess, updatedRefresh string
	manager.SetTokenUpdateCallback(func(access, refresh string) {
		updatedAccess = access
		updatedRefresh = refresh
	})

	err := manager.Refresh(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "new-access-token", manager.GetAccessToken())
	// Check that callback was called with new tokens
	assert.Equal(t, "new-access-token", updatedAccess)
	assert.Equal(t, "new-refresh-token", updatedRefresh)
}

func TestOAuth2Manager_Refresh_NoRefreshToken(t *testing.T) {
	manager := NewOAuth2Manager("cid", "secret", "access", "")
	err := manager.Refresh(context.Background())
	assert.Error(t, err)
	assert.True(t, errors.IsInvalidCredentials(err))
}

func TestOAuth2Manager_Refresh_Non200Response(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	manager := newTestOAuth2Manager(t, server.URL)
	err := manager.Refresh(context.Background())
	assert.Error(t, err)
	assert.True(t, errors.IsInvalidCredentials(err))
}

func TestOAuth2Manager_Refresh_NetworkError(t *testing.T) {
	manager := NewOAuth2Manager("cid", "secret", "access", "refresh")
	manager.SetTransport(&errorTransport{})
	err := manager.Refresh(context.Background())
	assert.Error(t, err)
	assert.True(t, isNetworkTimeout(err))
}

func TestOAuth2Manager_Refresh_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("Request path: %s", r.URL.Path)
		assert.Equal(t, "/api/oauth2/token", r.URL.Path)
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))
		// parse form data to ensure request is valid
		if err := r.ParseForm(); err != nil {
			t.Logf("ParseForm error: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		t.Logf("Form data: grant_type=%s, refresh_token=%s, client_id=%s, client_secret=%s",
			r.Form.Get("grant_type"), r.Form.Get("refresh_token"), r.Form.Get("client_id"), r.Form.Get("client_secret"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{invalid json`))
	}))
	defer server.Close()

	manager := newTestOAuth2Manager(t, server.URL)
	err := manager.Refresh(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "decode token response")
}
