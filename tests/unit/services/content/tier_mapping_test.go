package content_test

import (
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/services/content"
	"github.com/stretchr/testify/assert"
)

func TestNewTierMapping(t *testing.T) {
	assert.IsType(t, &content.LinearTierMapping{}, content.NewTierMapping("linear"))
	assert.IsType(t, &content.ModularTierMapping{}, content.NewTierMapping("modular"))
	assert.IsType(t, &content.ExclusiveTierMapping{}, content.NewTierMapping("exclusive"))
	assert.IsType(t, &content.LinearTierMapping{}, content.NewTierMapping("unknown"))
}

func TestLinearTierMapping(t *testing.T) {
	m := &content.LinearTierMapping{}
	tiers := []content.TierInfo{
		{ID: "tier1", AmountCents: 500},
		{ID: "tier2", AmountCents: 1000},
	}
	assert.Equal(t, "tier1", m.MapTier(100, 50, tiers))
	assert.Equal(t, "tier1", m.MapTier(0, 0, tiers))
}

func TestLinearTierMapping_EmptyTiers(t *testing.T) {
	m := &content.LinearTierMapping{}
	assert.Equal(t, "", m.MapTier(100, 50, nil))
}

func TestModularTierMapping(t *testing.T) {
	m := &content.ModularTierMapping{}
	tiers := []content.TierInfo{
		{ID: "tier1", AmountCents: 500},
		{ID: "tier2", AmountCents: 1000},
		{ID: "tier3", AmountCents: 2000},
	}

	assert.Equal(t, "tier1", m.MapTier(10, 5, tiers))
	assert.Equal(t, "tier2", m.MapTier(100, 10, tiers))
	assert.Equal(t, "tier3", m.MapTier(1000, 100, tiers))
}

func TestModularTierMapping_EmptyTiers(t *testing.T) {
	m := &content.ModularTierMapping{}
	assert.Equal(t, "", m.MapTier(100, 50, nil))
}

func TestExclusiveTierMapping(t *testing.T) {
	m := content.NewExclusiveTierMapping()
	tiers := []content.TierInfo{
		{ID: "tier1", AmountCents: 500},
		{ID: "tier2", AmountCents: 1000},
	}
	assert.Equal(t, "tier1", m.MapTier(0, 0, tiers))
}

func TestTierMapper(t *testing.T) {
	mapper := content.NewTierMapper("linear")
	tiers := []content.TierInfo{
		{ID: "tier1", AmountCents: 500},
	}
	result := mapper.Map(100, 50, tiers)
	assert.Equal(t, "tier1", result)
}

func TestTierMapper_SetStrategy(t *testing.T) {
	mapper := content.NewTierMapper("linear")
	mapper.SetStrategy("modular")
	tiers := []content.TierInfo{
		{ID: "tier1", AmountCents: 500},
	}
	result := mapper.Map(100, 50, tiers)
	assert.NotEmpty(t, result)
}
