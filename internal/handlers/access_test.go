package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// mockTierGater is a mock for access.TierGater
type mockTierGater struct {
	mock.Mock
}

func (m *mockTierGater) VerifyAccess(ctx context.Context, patronID, contentID, requiredTier string, patronTiers []string) (bool, string, error) {
	args := m.Called(ctx, patronID, contentID, requiredTier, patronTiers)
	return args.Bool(0), args.String(1), args.Error(2)
}

// mockSignedURLGenerator is a mock for access.SignedURLGenerator
type mockSignedURLGenerator struct {
	mock.Mock
}

func (m *mockSignedURLGenerator) VerifySignedURL(token, contentID, sub string, expires int64) bool {
	args := m.Called(token, contentID, sub, expires)
	return args.Bool(0)
}

func TestAccessHandler_Download_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockURLGen := &mockSignedURLGenerator{}
	mockURLGen.On("VerifySignedURL", "valid-token", "content123", "user456", int64(1234567890)).Return(true)

	handler := NewAccessHandler(nil, mockURLGen, slog.Default())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "content_id", Value: "content123"}}
	c.Request = httptest.NewRequest("GET", "/download/content123?token=valid-token&sub=user456&exp=1234567890", nil)

	handler.Download(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"content_id":"content123","status":"download_ready"}`, w.Body.String())
	mockURLGen.AssertExpectations(t)
}

func TestAccessHandler_Download_MissingParams(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewAccessHandler(nil, nil, slog.Default())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "content_id", Value: "content123"}}
	c.Request = httptest.NewRequest("GET", "/download/content123", nil)

	handler.Download(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.JSONEq(t, `{"error":"missing token parameters"}`, w.Body.String())
}

func TestAccessHandler_Download_InvalidExpiry(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewAccessHandler(nil, nil, slog.Default())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "content_id", Value: "content123"}}
	c.Request = httptest.NewRequest("GET", "/download/content123?token=abc&sub=def&exp=invalid", nil)

	handler.Download(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.JSONEq(t, `{"error":"invalid expiry"}`, w.Body.String())
}

func TestAccessHandler_Download_InvalidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockURLGen := &mockSignedURLGenerator{}
	mockURLGen.On("VerifySignedURL", "invalid-token", "content123", "user456", int64(1234567890)).Return(false)

	handler := NewAccessHandler(nil, mockURLGen, slog.Default())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "content_id", Value: "content123"}}
	c.Request = httptest.NewRequest("GET", "/download/content123?token=invalid-token&sub=user456&exp=1234567890", nil)

	handler.Download(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.JSONEq(t, `{"error":"invalid or expired token"}`, w.Body.String())
	mockURLGen.AssertExpectations(t)
}

func TestAccessHandler_CheckAccess_SuccessWithAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockGater := &mockTierGater{}
	mockGater.On("VerifyAccess", mock.Anything, "patron123", "content456", "gold", mock.Anything).
		Return(true, "", nil)

	handler := NewAccessHandler(mockGater, nil, slog.Default())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "content_id", Value: "content456"}}
	c.Request = httptest.NewRequest("GET", "/access/content456?patron_id=patron123&required_tier=gold", nil)

	handler.CheckAccess(c)

	assert.Equal(t, http.StatusOK, w.Code)
	expected := `{"access":true,"content_id":"content456","required_tier":"gold"}`
	assert.JSONEq(t, expected, w.Body.String())
	mockGater.AssertExpectations(t)
}

func TestAccessHandler_CheckAccess_NoAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockGater := &mockTierGater{}
	mockGater.On("VerifyAccess", mock.Anything, "patron123", "content456", "gold", mock.Anything).
		Return(false, "https://upgrade.example.com", nil)

	handler := NewAccessHandler(mockGater, nil, slog.Default())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "content_id", Value: "content456"}}
	c.Request = httptest.NewRequest("GET", "/access/content456?patron_id=patron123&required_tier=gold", nil)

	handler.CheckAccess(c)

	assert.Equal(t, http.StatusOK, w.Code)
	expected := `{"access":false,"content_id":"content456","required_tier":"gold","upgrade_url":"https://upgrade.example.com"}`
	assert.JSONEq(t, expected, w.Body.String())
	mockGater.AssertExpectations(t)
}

func TestAccessHandler_CheckAccess_MissingParams(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewAccessHandler(nil, nil, slog.Default())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "content_id", Value: "content456"}}
	c.Request = httptest.NewRequest("GET", "/access/content456", nil)

	handler.CheckAccess(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.JSONEq(t, `{"error":"missing patron_id or required_tier"}`, w.Body.String())
}
