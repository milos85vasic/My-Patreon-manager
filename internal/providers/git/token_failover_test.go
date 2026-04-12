package git

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenManager_NewTokenManager(t *testing.T) {
	tm := NewTokenManager("primary", "secondary")
	assert.Equal(t, "primary", tm.PrimaryToken)
	assert.Equal(t, "secondary", tm.SecondaryToken)
	assert.Equal(t, "primary", tm.CurrentToken)
	assert.NotNil(t, tm.cb)
}

func TestTokenManager_GetCurrentToken(t *testing.T) {
	tm := NewTokenManager("primary", "secondary")
	assert.Equal(t, "primary", tm.GetCurrentToken())
	tm.CurrentToken = "secondary"
	assert.Equal(t, "secondary", tm.GetCurrentToken())
}

func TestTokenManager_Failover(t *testing.T) {
	t.Run("primary to secondary", func(t *testing.T) {
		tm := NewTokenManager("primary", "secondary")
		var from, to string
		tm.OnFailover = func(f, t string) { from = f; to = t }
		err := tm.Failover()
		assert.NoError(t, err)
		assert.Equal(t, "primary", from)
		assert.Equal(t, "secondary", to)
		assert.Equal(t, "secondary", tm.CurrentToken)
	})

	t.Run("secondary to primary", func(t *testing.T) {
		tm := NewTokenManager("primary", "secondary")
		tm.CurrentToken = "secondary"
		var from, to string
		tm.OnFailover = func(f, t string) { from = f; to = t }
		err := tm.Failover()
		assert.NoError(t, err)
		assert.Equal(t, "secondary", from)
		assert.Equal(t, "primary", to)
		assert.Equal(t, "primary", tm.CurrentToken)
	})

	t.Run("no secondary token", func(t *testing.T) {
		tm := NewTokenManager("primary", "")
		err := tm.Failover()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no secondary token available")
		assert.Equal(t, "primary", tm.CurrentToken) // unchanged
	})
}

func TestTokenManager_IsFailedOver(t *testing.T) {
	tm := NewTokenManager("primary", "secondary")
	assert.False(t, tm.IsFailedOver())
	tm.CurrentToken = "secondary"
	assert.True(t, tm.IsFailedOver())
}

func TestRateLimitAwareClient_Do(t *testing.T) {
	t.Run("no token", func(t *testing.T) {
		tm := NewTokenManager("", "")
		client := NewRateLimitAwareClient(tm, "http://example.com")
		req := httptest.NewRequest("GET", "http://example.com", nil)
		_, err := client.Do(req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no token available")
	})

	t.Run("with token", func(t *testing.T) {
		// Create a test server that echoes a header
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		}))
		defer server.Close()

		tm := NewTokenManager("test-token", "")
		client := NewRateLimitAwareClient(tm, server.URL)
		req, err := http.NewRequest("GET", server.URL, nil)
		require.NoError(t, err)
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

func TestRateLimitAwareClient_HandleRateLimit(t *testing.T) {
	t.Run("rate limited", func(t *testing.T) {
		tm := NewTokenManager("primary", "secondary")
		var failoverCalled bool
		tm.OnFailover = func(_, _ string) { failoverCalled = true }
		client := NewRateLimitAwareClient(tm, "")
		resp := &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header:     make(http.Header),
		}
		err := client.HandleRateLimit(resp)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "rate limited")
		assert.True(t, failoverCalled)
	})

	t.Run("forbidden", func(t *testing.T) {
		tm := NewTokenManager("primary", "secondary")
		var failoverCalled bool
		tm.OnFailover = func(_, _ string) { failoverCalled = true }
		client := NewRateLimitAwareClient(tm, "")
		resp := &http.Response{StatusCode: http.StatusForbidden}
		err := client.HandleRateLimit(resp)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "token rejected")
		assert.True(t, failoverCalled)
	})

	t.Run("unauthorized", func(t *testing.T) {
		tm := NewTokenManager("primary", "secondary")
		var failoverCalled bool
		tm.OnFailover = func(_, _ string) { failoverCalled = true }
		client := NewRateLimitAwareClient(tm, "")
		resp := &http.Response{StatusCode: http.StatusUnauthorized}
		err := client.HandleRateLimit(resp)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "token rejected")
		assert.True(t, failoverCalled)
	})

	t.Run("no error", func(t *testing.T) {
		tm := NewTokenManager("primary", "secondary")
		client := NewRateLimitAwareClient(tm, "")
		resp := &http.Response{StatusCode: http.StatusOK}
		err := client.HandleRateLimit(resp)
		assert.NoError(t, err)
	})
}

func TestOAuth2TokenManager_New(t *testing.T) {
	m := NewOAuth2TokenManager("client-id", "client-secret", "access", "refresh")
	assert.Equal(t, "client-id", m.ClientID)
	assert.Equal(t, "client-secret", m.ClientSecret)
	assert.Equal(t, "access", m.AccessToken)
	assert.Equal(t, "refresh", m.RefreshToken)
	assert.False(t, m.IsExpired())
	assert.NotNil(t, m.client)
}

func TestOAuth2TokenManager_GetAccessToken(t *testing.T) {
	m := NewOAuth2TokenManager("", "", "token", "")
	assert.Equal(t, "token", m.GetAccessToken())
}

func TestOAuth2TokenManager_IsExpired(t *testing.T) {
	m := NewOAuth2TokenManager("", "", "", "")
	m.ExpiresAt = time.Now().Add(-1 * time.Second)
	assert.True(t, m.IsExpired())
	m.ExpiresAt = time.Now().Add(1 * time.Hour)
	assert.False(t, m.IsExpired())
}

func TestOAuth2TokenManager_Refresh(t *testing.T) {
	t.Run("no refresh token", func(t *testing.T) {
		m := NewOAuth2TokenManager("client", "secret", "access", "")
		err := m.Refresh(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no refresh token available")
	})

	t.Run("successful refresh", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{}`))
		}))
		defer server.Close()

		m := NewOAuth2TokenManager("client", "secret", "access", "refresh")
		m.RefreshURL = server.URL
		var updatedAccess, updatedRefresh string
		m.OnTokenUpdate = func(a, r string) { updatedAccess = a; updatedRefresh = r }

		err := m.Refresh(context.Background())
		assert.NoError(t, err)
		assert.False(t, m.IsExpired())
		assert.Equal(t, "access", updatedAccess)
		assert.Equal(t, "refresh", updatedRefresh)
	})

	t.Run("network error", func(t *testing.T) {
		m := NewOAuth2TokenManager("client", "secret", "access", "refresh")
		// Point at a server that immediately closes connections
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		server.Close() // close immediately to force network error
		m.RefreshURL = server.URL

		err := m.Refresh(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "token refresh failed")
	})

	t.Run("non-200 response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer server.Close()

		m := NewOAuth2TokenManager("client", "secret", "access", "refresh")
		m.RefreshURL = server.URL

		err := m.Refresh(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "token refresh returned 400")
	})
}

func TestOAuth2TokenManager_ConcurrentAccess(t *testing.T) {
	m := NewOAuth2TokenManager("client", "secret", "access", "refresh")
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.GetAccessToken()
			_ = m.IsExpired()
		}()
	}
	wg.Wait()
	// No assertion, just ensure no panic
}
