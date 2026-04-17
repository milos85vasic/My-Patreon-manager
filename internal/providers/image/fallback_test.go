package image

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFallbackProvider_Name(t *testing.T) {
	p := NewFallbackProvider()
	assert.Equal(t, "fallback", p.ProviderName())
}

func TestFallbackProvider_SingleProviderSuccess(t *testing.T) {
	fp := NewFallbackProvider(
		&mockProvider{name: "dalle", available: true, result: &ImageResult{URL: "https://img.png", Provider: "dalle"}},
	)
	result, err := fp.GenerateImage(context.Background(), ImageRequest{Prompt: "test"})
	require.NoError(t, err)
	assert.Equal(t, "dalle", result.Provider)
}

func TestFallbackProvider_FallbackOnFailure(t *testing.T) {
	fp := NewFallbackProvider(
		&mockProvider{name: "dalle", available: true, err: errors.New("rate limited")},
		&mockProvider{name: "stability", available: true, result: &ImageResult{URL: "https://img.png", Provider: "stability"}},
	)
	result, err := fp.GenerateImage(context.Background(), ImageRequest{Prompt: "test"})
	require.NoError(t, err)
	assert.Equal(t, "stability", result.Provider)
}

func TestFallbackProvider_AllFail(t *testing.T) {
	fp := NewFallbackProvider(
		&mockProvider{name: "dalle", available: true, err: errors.New("fail 1")},
		&mockProvider{name: "stability", available: true, err: errors.New("fail 2")},
	)
	result, err := fp.GenerateImage(context.Background(), ImageRequest{Prompt: "test"})
	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "all providers failed")
}

func TestFallbackProvider_SkipsUnavailable(t *testing.T) {
	fp := NewFallbackProvider(
		&mockProvider{name: "dalle", available: false},
		&mockProvider{name: "stability", available: true, result: &ImageResult{URL: "https://img.png", Provider: "stability"}},
	)
	result, err := fp.GenerateImage(context.Background(), ImageRequest{Prompt: "test"})
	require.NoError(t, err)
	assert.Equal(t, "stability", result.Provider)
}

func TestFallbackProvider_IsAvailable(t *testing.T) {
	fp := NewFallbackProvider(
		&mockProvider{name: "dalle", available: false},
		&mockProvider{name: "stability", available: true},
	)
	assert.True(t, fp.IsAvailable(context.Background()))

	fp2 := NewFallbackProvider(
		&mockProvider{name: "dalle", available: false},
		&mockProvider{name: "stability", available: false},
	)
	assert.False(t, fp2.IsAvailable(context.Background()))
}

func TestFallbackProvider_NoProviders(t *testing.T) {
	fp := NewFallbackProvider()
	assert.False(t, fp.IsAvailable(context.Background()))

	result, err := fp.GenerateImage(context.Background(), ImageRequest{Prompt: "test"})
	assert.Nil(t, result)
	assert.Error(t, err)
}

func TestFallbackProvider_SetLogger(t *testing.T) {
	fp := NewFallbackProvider()
	fp.SetLogger(nil)
	assert.NotNil(t, fp)
}
