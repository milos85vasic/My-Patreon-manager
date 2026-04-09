package content

import (
	"sync"
)

type TierMappingStrategy interface {
	MapTier(repoStars, repoForks int, allTiers []TierInfo) string
}

type TierInfo struct {
	ID          string
	Title       string
	AmountCents int
}

type LinearTierMapping struct{}

func (m *LinearTierMapping) MapTier(repoStars, repoForks int, allTiers []TierInfo) string {
	if len(allTiers) == 0 {
		return ""
	}
	return allTiers[0].ID
}

type ModularTierMapping struct{}

func (m *ModularTierMapping) MapTier(repoStars, repoForks int, allTiers []TierInfo) string {
	if len(allTiers) == 0 {
		return ""
	}
	score := float64(repoStars)*1.0 + float64(repoForks)*0.5
	switch {
	case score >= 1000 && len(allTiers) > 2:
		return allTiers[len(allTiers)-1].ID
	case score >= 100 && len(allTiers) > 1:
		return allTiers[len(allTiers)/2].ID
	default:
		return allTiers[0].ID
	}
}

type ExclusiveTierMapping struct {
	tierThresholds map[string]int
}

func NewExclusiveTierMapping() *ExclusiveTierMapping {
	return &ExclusiveTierMapping{
		tierThresholds: make(map[string]int),
	}
}

func (m *ExclusiveTierMapping) MapTier(repoStars, repoForks int, allTiers []TierInfo) string {
	score := repoStars + repoForks
	for _, tier := range allTiers {
		if threshold, ok := m.tierThresholds[tier.ID]; ok {
			if score >= threshold {
				return tier.ID
			}
		}
	}
	if len(allTiers) > 0 {
		return allTiers[0].ID
	}
	return ""
}

func NewTierMapping(strategy string) TierMappingStrategy {
	switch strategy {
	case "modular":
		return &ModularTierMapping{}
	case "exclusive":
		return &ExclusiveTierMapping{}
	default:
		return &LinearTierMapping{}
	}
}

type TierMapper struct {
	strategy TierMappingStrategy
	mu       sync.RWMutex
}

func NewTierMapper(strategy string) *TierMapper {
	return &TierMapper{
		strategy: NewTierMapping(strategy),
	}
}

func (m *TierMapper) Map(repoStars, repoForks int, tiers []TierInfo) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.strategy.MapTier(repoStars, repoForks, tiers)
}

func (m *TierMapper) SetStrategy(strategy string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.strategy = NewTierMapping(strategy)
}
