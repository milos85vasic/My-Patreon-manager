package errors

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewProviderError(t *testing.T) {
	resetTime := time.Now().Add(5 * time.Minute)
	err := NewProviderError("test message", "test_code", true, resetTime)

	assert.Equal(t, "test message", err.Error())
	assert.Equal(t, "test_code", err.Code())
	assert.True(t, err.Retryable())
	assert.Equal(t, resetTime, err.RateLimitReset())
}

func TestInvalidCredentials(t *testing.T) {
	err := InvalidCredentials("invalid credentials")
	assert.Equal(t, "invalid credentials", err.Error())
	assert.Equal(t, "invalid_credentials", err.Code())
	assert.False(t, err.Retryable())
	assert.True(t, err.RateLimitReset().IsZero())
}

func TestNetworkTimeout(t *testing.T) {
	err := NetworkTimeout("network timeout")
	assert.Equal(t, "network timeout", err.Error())
	assert.Equal(t, "network_timeout", err.Code())
	assert.True(t, err.Retryable())
	assert.True(t, err.RateLimitReset().IsZero())
}

func TestRateLimited(t *testing.T) {
	resetTime := time.Now().Add(30 * time.Second)
	err := RateLimited("rate limited", resetTime)
	assert.Equal(t, "rate limited", err.Error())
	assert.Equal(t, "rate_limited", err.Code())
	assert.True(t, err.Retryable())
	assert.Equal(t, resetTime, err.RateLimitReset())
}

func TestPermissionDenied(t *testing.T) {
	err := PermissionDenied("permission denied")
	assert.Equal(t, "permission denied", err.Error())
	assert.Equal(t, "permission_denied", err.Code())
	assert.False(t, err.Retryable())
	assert.True(t, err.RateLimitReset().IsZero())
}

func TestNotFound(t *testing.T) {
	err := NotFound("not found")
	assert.Equal(t, "not found", err.Error())
	assert.Equal(t, "not_found", err.Code())
	assert.False(t, err.Retryable())
	assert.True(t, err.RateLimitReset().IsZero())
}

func TestRenderingFailed(t *testing.T) {
	err := RenderingFailed("rendering failed")
	assert.Equal(t, "rendering failed", err.Error())
	assert.Equal(t, "rendering_failed", err.Code())
	assert.False(t, err.Retryable())
	assert.True(t, err.RateLimitReset().IsZero())
}

func TestTimeout(t *testing.T) {
	err := Timeout("timeout")
	assert.Equal(t, "timeout", err.Error())
	assert.Equal(t, "timeout", err.Code())
	assert.True(t, err.Retryable())
	assert.True(t, err.RateLimitReset().IsZero())
}

func TestLockContention(t *testing.T) {
	err := LockContention("lock contention")
	assert.Equal(t, "lock contention", err.Error())
	assert.Equal(t, "lock_contention", err.Code())
	assert.False(t, err.Retryable())
	assert.True(t, err.RateLimitReset().IsZero())
}

func TestIsRateLimited(t *testing.T) {
	t.Run("rate limited error", func(t *testing.T) {
		resetTime := time.Now()
		err := RateLimited("rate limited", resetTime)
		assert.True(t, IsRateLimited(err))
	})

	t.Run("other provider error", func(t *testing.T) {
		err := InvalidCredentials("invalid")
		assert.False(t, IsRateLimited(err))
	})

	t.Run("standard error", func(t *testing.T) {
		err := errors.New("standard error")
		assert.False(t, IsRateLimited(err))
	})

	t.Run("nil error", func(t *testing.T) {
		assert.False(t, IsRateLimited(nil))
	})
}

func TestIsLockContention(t *testing.T) {
	t.Run("lock contention error", func(t *testing.T) {
		err := LockContention("lock contention")
		assert.True(t, IsLockContention(err))
	})

	t.Run("other provider error", func(t *testing.T) {
		err := InvalidCredentials("invalid")
		assert.False(t, IsLockContention(err))
	})

	t.Run("standard error", func(t *testing.T) {
		err := errors.New("standard error")
		assert.False(t, IsLockContention(err))
	})

	t.Run("nil error", func(t *testing.T) {
		assert.False(t, IsLockContention(nil))
	})
}

func TestIsInvalidCredentials(t *testing.T) {
	t.Run("invalid credentials error", func(t *testing.T) {
		err := InvalidCredentials("invalid")
		assert.True(t, IsInvalidCredentials(err))
	})

	t.Run("other provider error", func(t *testing.T) {
		err := NetworkTimeout("timeout")
		assert.False(t, IsInvalidCredentials(err))
	})

	t.Run("standard error", func(t *testing.T) {
		err := errors.New("standard error")
		assert.False(t, IsInvalidCredentials(err))
	})

	t.Run("nil error", func(t *testing.T) {
		assert.False(t, IsInvalidCredentials(nil))
	})
}
