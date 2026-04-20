package llm

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestNewGatewayProvider_DefaultsEmptyModel confirms the constructor
// substitutes a default model identifier when the caller passes "".
func TestNewGatewayProvider_DefaultsEmptyModel(t *testing.T) {
	p := NewGatewayProvider(nil, nil, nil, "")
	if assert.NotNil(t, p) {
		assert.Equal(t, "llama-3.3-70b-versatile", p.model)
	}
}

// TestNewGatewayProvider_HonoursExplicitModel keeps the caller-supplied
// default intact when non-empty.
func TestNewGatewayProvider_HonoursExplicitModel(t *testing.T) {
	p := NewGatewayProvider(nil, nil, nil, "custom-model-42")
	if assert.NotNil(t, p) {
		assert.Equal(t, "custom-model-42", p.model)
	}
}

// TestGatewayProvider_NilVerifierPaths asserts the three delegating
// methods return safe defaults when no VerifierClient is wired (rather
// than panicking on a nil dereference).
func TestGatewayProvider_NilVerifierPaths(t *testing.T) {
	p := NewGatewayProvider(nil, nil, nil, "")
	ctx := context.Background()

	models, err := p.GetAvailableModels(ctx)
	assert.NoError(t, err)
	assert.Nil(t, models)

	score, err := p.GetModelQualityScore(ctx, "irrelevant")
	assert.NoError(t, err)
	assert.InDelta(t, 0.5, score, 0.0001)

	usage, err := p.GetTokenUsage(ctx)
	assert.NoError(t, err)
	assert.Zero(t, usage)
}

// TestExtractTitle covers every branch of the first-line title extractor:
// plain text, markdown heading, long truncation, pure-# heading, empty
// input, and no-newline input.
func TestExtractTitle(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"plain first line", "Hello world\nthen body", "Hello world"},
		{"markdown heading", "# Topic Title\n\nbody", "Topic Title"},
		{"deep heading", "### Deep dive\nbody", "Deep dive"},
		{"truncates long title", strings.Repeat("x", 150) + "\nbody", strings.Repeat("x", 100)},
		{"empty content falls back", "", "Generated Content"},
		{"no newline falls back", "single line without terminator", "Generated Content"},
		{"hashes-only heading falls back", "###\nbody", "Generated Content"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, extractTitle(tc.content))
		})
	}
}

// TestEstimateQuality covers the clamping logic: empty → 0, very short
// body → clamped floor of 0.3, mid-length → proportional, very long →
// clamped ceiling of 1.0.
func TestEstimateQuality(t *testing.T) {
	assert.InDelta(t, 0.0, estimateQuality("", 0), 0.0001, "empty content gets zero score")
	assert.InDelta(t, 0.3, estimateQuality("short", 10), 0.0001, "short content clamps to floor 0.3")
	mid := strings.Repeat("a", 1500)
	assert.InDelta(t, 0.5, estimateQuality(mid, 200), 0.0001, "mid-length content scales linearly")
	long := strings.Repeat("a", 5000)
	assert.InDelta(t, 1.0, estimateQuality(long, 200), 0.0001, "long content clamps to ceiling 1.0")
}
