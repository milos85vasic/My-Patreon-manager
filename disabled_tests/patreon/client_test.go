//go:build disabled

//go:build disabled\n
package patreon_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/errors"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/patreon"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	_ = errors.IsInvalidCredentials
	_ = models.Campaign{}
)

// newTestClient creates a Patreon client configured to use the test server's base URL.
func newTestClient(t *testing.T, serverURL string) *patreon.Client {
	t.Helper()
	oauth := patreon.NewOAuth2Manager("test-client-id", "test-client-secret", "test-access-token", "test-refresh-token")
	oauth.SetTokenEndpoint(serverURL + "/api/oauth2/token")
	client := patreon.NewClient(oauth, "test-campaign-id")
	client.SetBaseURL(serverURL)
	return client
}

func TestClient_GetCampaign_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/campaigns/test-campaign-id?fields[campaign]=name,summary,patron_count", r.URL.Path+"?"+r.URL.RawQuery)
		assert.Equal(t, "Bearer test-access-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"id": "test-campaign-id",
				"attributes": map[string]interface{}{
					"name":         "My Campaign",
					"summary":      "A test campaign",
					"patron_count": 42,
				},
			},
		})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	campaign, err := client.GetCampaign(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "test-campaign-id", campaign.ID)
	assert.Equal(t, "My Campaign", campaign.Name)
	assert.Equal(t, "A test campaign", campaign.Summary)
	assert.Equal(t, 42, campaign.PatronCount)
}

func TestClient_ListTiers_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/campaigns/test-campaign-id/tiers?fields[tier]=title,description,amount_cents,patron_count", r.URL.Path+"?"+r.URL.RawQuery)
		assert.Equal(t, "Bearer test-access-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{
					"id": "tier-1",
					"attributes": map[string]interface{}{
						"title":        "Silver",
						"description":  "Silver tier",
						"amount_cents": 500,
						"patron_count": 10,
					},
				},
				{
					"id": "tier-2",
					"attributes": map[string]interface{}{
						"title":        "Gold",
						"description":  "Gold tier",
						"amount_cents": 1000,
						"patron_count": 5,
					},
				},
			},
		})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	tiers, err := client.ListTiers(context.Background())
	require.NoError(t, err)
	require.Len(t, tiers, 2)
	assert.Equal(t, "tier-1", tiers[0].ID)
	assert.Equal(t, "test-campaign-id", tiers[0].CampaignID)
	assert.Equal(t, "Silver", tiers[0].Title)
	assert.Equal(t, "Silver tier", tiers[0].Description)
	assert.Equal(t, 500, tiers[0].AmountCents)
	assert.Equal(t, 10, tiers[0].PatronCount)
	assert.Equal(t, "tier-2", tiers[1].ID)
	assert.Equal(t, "test-campaign-id", tiers[1].CampaignID)
	assert.Equal(t, "Gold", tiers[1].Title)
	assert.Equal(t, "Gold tier", tiers[1].Description)
	assert.Equal(t, 1000, tiers[1].AmountCents)
	assert.Equal(t, 5, tiers[1].PatronCount)
}

func TestClient_CreatePost_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/posts", r.URL.Path)
		assert.Equal(t, "Bearer test-access-token", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		// Decode request body
		var reqBody map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		require.NoError(t, err)
		// Validate structure
		data := reqBody["data"].(map[string]interface{})
		assert.Equal(t, "post", data["type"])
		attrs := data["attributes"].(map[string]interface{})
		assert.Equal(t, "Test Post", attrs["title"])
		assert.Equal(t, "Test content", attrs["content"])
		assert.Equal(t, "text", attrs["post_type"])
		assert.Equal(t, true, attrs["is_paid"])
		assert.Equal(t, "draft", attrs["publication_type"])
		relationships := data["relationships"].(map[string]interface{})
		campaign := relationships["campaign"].(map[string]interface{})
		campaignData := campaign["data"].(map[string]interface{})
		assert.Equal(t, "campaign", campaignData["type"])
		assert.Equal(t, "test-campaign-id", campaignData["id"])
		// Respond
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"id": "new-post-id",
			},
		})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	post := &models.Post{
		Title:    "Test Post",
		Content:  "Test content",
		PostType: "text",
	}
	result, err := client.CreatePost(context.Background(), post)
	require.NoError(t, err)
	assert.Equal(t, "new-post-id", result.ID)
	assert.Equal(t, "draft", result.PublicationStatus)
}

func TestClient_UpdatePost_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/posts/existing-post-id", r.URL.Path)
		assert.Equal(t, "PATCH", r.Method)
		assert.Equal(t, "Bearer test-access-token", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		// Decode request body
		var reqBody map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		require.NoError(t, err)
		data := reqBody["data"].(map[string]interface{})
		assert.Equal(t, "post", data["type"])
		assert.Equal(t, "existing-post-id", data["id"])
		attrs := data["attributes"].(map[string]interface{})
		assert.Equal(t, "Updated Title", attrs["title"])
		assert.Equal(t, "Updated content", attrs["content"])
		// Respond with empty OK
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	post := &models.Post{
		ID:      "existing-post-id",
		Title:   "Updated Title",
		Content: "Updated content",
	}
	result, err := client.UpdatePost(context.Background(), post)
	require.NoError(t, err)
	assert.Same(t, post, result) // returns same pointer
}

func TestClient_DeletePost_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/posts/post-to-delete", r.URL.Path)
		assert.Equal(t, "DELETE", r.Method)
		assert.Equal(t, "Bearer test-access-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.DeletePost(context.Background(), "post-to-delete")
	require.NoError(t, err)
}

func TestClient_AssociateTiers_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/posts/post-123", r.URL.Path)
		assert.Equal(t, "PATCH", r.Method)
		assert.Equal(t, "Bearer test-access-token", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		var reqBody map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		require.NoError(t, err)
		data := reqBody["data"].(map[string]interface{})
		assert.Equal(t, "post", data["type"])
		assert.Equal(t, "post-123", data["id"])
		relationships := data["relationships"].(map[string]interface{})
		tiers := relationships["tiers"].(map[string]interface{})
		tierData := tiers["data"].([]interface{})
		require.Len(t, tierData, 2)
		assert.Equal(t, map[string]interface{}{"type": "tier", "id": "tier-1"}, tierData[0])
		assert.Equal(t, map[string]interface{}{"type": "tier", "id": "tier-2"}, tierData[1])
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.AssociateTiers(context.Background(), "post-123", []string{"tier-1", "tier-2"})
	require.NoError(t, err)
}

func TestClient_GetCampaign_Unauthorized_RefreshSuccess(t *testing.T) {
	var requestCount int
	var refreshRequested bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		switch {
		case r.URL.Path == "/api/oauth2/token":
			// Token refresh request
			assert.Equal(t, "POST", r.Method)
			assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			vals, err := url.ParseQuery(string(body))
			require.NoError(t, err)
			assert.Equal(t, "refresh_token", vals.Get("grant_type"))
			assert.Equal(t, "test-refresh-token", vals.Get("refresh_token"))
			assert.Equal(t, "test-client-id", vals.Get("client_id"))
			assert.Equal(t, "test-client-secret", vals.Get("client_secret"))
			// Respond with new tokens
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token":  "new-access-token",
				"refresh_token": "new-refresh-token",
				"expires_in":    3600,
			})
			refreshRequested = true
		case r.URL.Path == "/campaigns/test-campaign-id":
			// API request
			if requestCount == 1 {
				// First request returns 401
				assert.Equal(t, "Bearer test-access-token", r.Header.Get("Authorization"))
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			// Second request after refresh
			assert.Equal(t, "Bearer new-access-token", r.Header.Get("Authorization"))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]interface{}{
					"id": "test-campaign-id",
					"attributes": map[string]interface{}{
						"name":         "My Campaign",
						"summary":      "A test campaign",
						"patron_count": 42,
					},
				},
			})
		default:
			t.Errorf("unexpected request path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	campaign, err := client.GetCampaign(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "test-campaign-id", campaign.ID)
	assert.True(t, refreshRequested, "token refresh should have been requested")
	assert.Equal(t, 3, requestCount) // 1: 401, 2: token refresh, 3: retry with new token
}

func TestClient_GetCampaign_RateLimited(t *testing.T) {
	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		// Always return 429
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.GetCampaign(context.Background())
	require.Error(t, err)
	assert.True(t, errors.IsRateLimited(err))
	// Should have retried up to maxRetries (3) then given up
	assert.Equal(t, 3, requestCount)
}

func TestClient_GetCampaign_NetworkError(t *testing.T) {
	// Custom transport that returns an error
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("simulated network failure")
	})
	client := newTestClient(t, "http://dummy") // base URL doesn't matter
	client.SetTransport(transport)
	_, err := client.GetCampaign(context.Background())
	require.Error(t, err)
	// Check that error is a ProviderError with code "network_timeout"
	var pe errors.ProviderError
	require.ErrorAs(t, err, &pe)
	assert.Equal(t, "network_timeout", pe.Code())
	assert.True(t, pe.Retryable())
}

func TestClient_GetCampaign_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/campaigns/test-campaign-id?fields[campaign]=name,summary,patron_count", r.URL.Path+"?"+r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data": { "id": "test", "attributes": { "name": "test" }`)) // missing closing braces
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.GetCampaign(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode campaign")
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
