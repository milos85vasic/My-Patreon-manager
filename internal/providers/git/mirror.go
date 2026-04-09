package git

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/utils"
)

type MirrorDetector struct{}

func NewMirrorDetector() *MirrorDetector { return &MirrorDetector{} }

func (d *MirrorDetector) DetectMirrors(repos []models.Repository) []models.MirrorMap {
	var mirrors []models.MirrorMap
	grouped := make(map[string][]models.Repository)

	for _, r := range repos {
		key := strings.ToLower(r.Owner + "/" + r.Name)
		grouped[key] = append(grouped[key], r)
	}

	checked := make(map[string]bool)
	for _, r1 := range repos {
		key1 := strings.ToLower(r1.Owner + "/" + r1.Name)
		if checked[key1] {
			continue
		}

		for _, r2 := range repos {
			if r1.Service == r2.Service {
				continue
			}
			key2 := strings.ToLower(r2.Owner + "/" + r2.Name)
			if checked[key2] {
				continue
			}

			confidence := d.computeSimilarity(r1, r2)
			if confidence >= 0.8 {
				groupID := utils.NewUUID()
				canonical := d.selectCanonical(r1, r2)

				mirrors = append(mirrors, models.MirrorMap{
					ID:              utils.NewUUID(),
					MirrorGroupID:   groupID,
					RepositoryID:    r1.ID,
					IsCanonical:     canonical == r1.ID,
					ConfidenceScore: confidence,
					DetectionMethod: "name_match",
				})
				mirrors = append(mirrors, models.MirrorMap{
					ID:              utils.NewUUID(),
					MirrorGroupID:   groupID,
					RepositoryID:    r2.ID,
					IsCanonical:     canonical == r2.ID,
					ConfidenceScore: confidence,
					DetectionMethod: "name_match",
				})

				checked[key1] = true
				checked[key2] = true
				break
			}
		}
	}

	return mirrors
}

func (d *MirrorDetector) computeSimilarity(r1, r2 models.Repository) float64 {
	score := 0.0

	if strings.ToLower(r1.Name) == strings.ToLower(r2.Name) {
		score += 0.5
	}

	if strings.ToLower(r1.Owner) == strings.ToLower(r2.Owner) {
		score += 0.2
	}

	if r1.READMEContent != "" && r2.READMEContent != "" {
		h1 := sha256.Sum256([]byte(r1.READMEContent))
		h2 := sha256.Sum256([]byte(r2.READMEContent))
		if fmt.Sprintf("%x", h1) == fmt.Sprintf("%x", h2) {
			score += 0.3
		}
	}

	if r1.Description != "" && r2.Description != "" {
		if strings.ToLower(r1.Description) == strings.ToLower(r2.Description) {
			score += 0.2
		}
	}

	if score > 1.0 {
		score = 1.0
	}
	return score
}

func (d *MirrorDetector) selectCanonical(r1, r2 models.Repository) string {
	serviceOrder := map[string]int{"github": 1, "gitlab": 2, "gitflic": 3, "gitverse": 4}
	p1, ok1 := serviceOrder[r1.Service]
	p2, ok2 := serviceOrder[r2.Service]
	if !ok1 {
		p1 = 99
	}
	if !ok2 {
		p2 = 99
	}
	if p1 <= p2 {
		return r1.ID
	}
	return r2.ID
}

func DetectMirrors(ctx context.Context, repos []models.Repository) ([]models.MirrorMap, error) {
	detector := NewMirrorDetector()
	return detector.DetectMirrors(repos), nil
}
