package content_test

import (
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/content"
	"github.com/stretchr/testify/assert"
)

func TestNewQualityGate(t *testing.T) {
	gate := content.NewQualityGate(0.75)
	assert.NotNil(t, gate)
}

func TestQualityGate_EvaluateQuality(t *testing.T) {
	gate := content.NewQualityGate(0.75)

	score, passed := gate.EvaluateQuality("test content", 0.9)
	assert.Equal(t, 0.9, score)
	assert.True(t, passed)

	score, passed = gate.EvaluateQuality("test content", 0.5)
	assert.Equal(t, 0.5, score)
	assert.False(t, passed)

	score, passed = gate.EvaluateQuality("test content", 0.75)
	assert.Equal(t, 0.75, score)
	assert.True(t, passed)
}

func TestQualityGate_Evaluate(t *testing.T) {
	gate := content.NewQualityGate(0.75)

	passed, score := gate.Evaluate(models.Content{
		Body:         "good content",
		QualityScore: 0.9,
	})
	assert.True(t, passed)
	assert.Equal(t, 0.9, score)

	passed, score = gate.Evaluate(models.Content{
		Body:         "bad content",
		QualityScore: 0.5,
	})
	assert.False(t, passed)
	assert.Equal(t, 0.5, score)
}

func TestQualityGate_ContentFingerprint(t *testing.T) {
	gate := content.NewQualityGate(0.75)

	fp1 := gate.ContentFingerprint("hello world")
	fp2 := gate.ContentFingerprint("hello world")
	fp3 := gate.ContentFingerprint("different content")

	assert.Equal(t, fp1, fp2)
	assert.NotEqual(t, fp1, fp3)
	assert.Len(t, fp1, 64)
}

func TestQualityGate_ThresholdBoundary(t *testing.T) {
	tests := []struct {
		name      string
		threshold float64
		score     float64
		expected  bool
	}{
		{"exact match", 0.75, 0.75, true},
		{"just above", 0.75, 0.751, true},
		{"just below", 0.75, 0.749, false},
		{"zero threshold", 0.0, 0.01, true},
		{"perfect score", 0.75, 1.0, true},
		{"zero score", 0.75, 0.0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gate := content.NewQualityGate(tt.threshold)
			_, passed := gate.EvaluateQuality("content", tt.score)
			assert.Equal(t, tt.expected, passed)
		})
	}
}
