package content

import (
	"crypto/sha256"
	"fmt"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

type QualityGate struct {
	threshold float64
}

func NewQualityGate(threshold float64) *QualityGate {
	return &QualityGate{threshold: threshold}
}

func (q *QualityGate) EvaluateQuality(content string, score float64) (float64, bool) {
	return score, score >= q.threshold
}

func (q *QualityGate) ContentFingerprint(body string) string {
	h := sha256.Sum256([]byte(body))
	return fmt.Sprintf("%x", h[:])
}

func (q *QualityGate) Evaluate(content models.Content) (bool, float64) {
	score := content.QualityScore
	return score >= q.threshold, score
}
