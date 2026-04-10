package errors

import "time"

type ProviderError interface {
	error
	Code() string
	Retryable() bool
	RateLimitReset() time.Time
}

type providerError struct {
	msg            string
	code           string
	retryable      bool
	rateLimitReset time.Time
}

func (e *providerError) Error() string             { return e.msg }
func (e *providerError) Code() string              { return e.code }
func (e *providerError) Retryable() bool           { return e.retryable }
func (e *providerError) RateLimitReset() time.Time { return e.rateLimitReset }

func NewProviderError(msg, code string, retryable bool, reset time.Time) ProviderError {
	return &providerError{msg: msg, code: code, retryable: retryable, rateLimitReset: reset}
}

func InvalidCredentials(msg string) ProviderError {
	return NewProviderError(msg, "invalid_credentials", false, time.Time{})
}

func NetworkTimeout(msg string) ProviderError {
	return NewProviderError(msg, "network_timeout", true, time.Time{})
}

func RateLimited(msg string, reset time.Time) ProviderError {
	return NewProviderError(msg, "rate_limited", true, reset)
}

func PermissionDenied(msg string) ProviderError {
	return NewProviderError(msg, "permission_denied", false, time.Time{})
}

func NotFound(msg string) ProviderError {
	return NewProviderError(msg, "not_found", false, time.Time{})
}

func RenderingFailed(msg string) ProviderError {
	return NewProviderError(msg, "rendering_failed", false, time.Time{})
}

func Timeout(msg string) ProviderError {
	return NewProviderError(msg, "timeout", true, time.Time{})
}

func LockContention(msg string) ProviderError {
	return NewProviderError(msg, "lock_contention", false, time.Time{})
}

func IsRateLimited(err error) bool {
	if pe, ok := err.(ProviderError); ok {
		return pe.Code() == "rate_limited"
	}
	return false
}

func IsLockContention(err error) bool {
	if pe, ok := err.(ProviderError); ok {
		return pe.Code() == "lock_contention"
	}
	return false
}

func IsInvalidCredentials(err error) bool {
	if pe, ok := err.(ProviderError); ok {
		return pe.Code() == "invalid_credentials"
	}
	return false
}
