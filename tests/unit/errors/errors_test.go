package errors_test

import (
	"testing"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/errors"
	"github.com/stretchr/testify/assert"
)

func TestProviderError_InvalidCredentials(t *testing.T) {
	err := errors.InvalidCredentials("bad creds")
	assert.Equal(t, "bad creds", err.Error())
	assert.Equal(t, "invalid_credentials", err.Code())
	assert.False(t, err.Retryable())
}

func TestProviderError_NetworkTimeout(t *testing.T) {
	err := errors.NetworkTimeout("timeout")
	assert.True(t, err.Retryable())
	assert.Equal(t, "network_timeout", err.Code())
}

func TestProviderError_RateLimited(t *testing.T) {
	reset := time.Now().Add(time.Hour)
	err := errors.RateLimited("too many requests", reset)
	assert.True(t, err.Retryable())
	assert.Equal(t, "rate_limited", err.Code())
	assert.Equal(t, reset, err.RateLimitReset())
}

func TestProviderError_PermissionDenied(t *testing.T) {
	err := errors.PermissionDenied("forbidden")
	assert.False(t, err.Retryable())
}

func TestProviderError_NotFound(t *testing.T) {
	err := errors.NotFound("missing")
	assert.Equal(t, "not_found", err.Code())
}

func TestProviderError_RenderingFailed(t *testing.T) {
	err := errors.RenderingFailed("pdf failed")
	assert.Equal(t, "rendering_failed", err.Code())
}

func TestProviderError_Timeout(t *testing.T) {
	err := errors.Timeout("timed out")
	assert.True(t, err.Retryable())
}

func TestProviderError_LockContention(t *testing.T) {
	err := errors.LockContention("locked")
	assert.False(t, err.Retryable())
	assert.Equal(t, "lock_contention", err.Code())
}

func TestIsRateLimited(t *testing.T) {
	err := errors.RateLimited("rate limited", time.Now())
	assert.True(t, errors.IsRateLimited(err))

	err2 := errors.NetworkTimeout("timeout")
	assert.False(t, errors.IsRateLimited(err2))

	assert.False(t, errors.IsRateLimited(nil))
}

func TestNewProviderError(t *testing.T) {
	err := errors.NewProviderError("msg", "code", true, time.Now())
	assert.Equal(t, "msg", err.Error())
	assert.Equal(t, "code", err.Code())
	assert.True(t, err.Retryable())
}
