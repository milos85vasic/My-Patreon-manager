package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRepository_TopicsJSON(t *testing.T) {
	t.Run("nil topics", func(t *testing.T) {
		r := &Repository{Topics: nil}
		result := r.TopicsJSON()
		assert.Equal(t, "null", result)
	})

	t.Run("empty topics", func(t *testing.T) {
		r := &Repository{Topics: []string{}}
		result := r.TopicsJSON()
		assert.Equal(t, "[]", result)
	})

	t.Run("with topics", func(t *testing.T) {
		r := &Repository{Topics: []string{"go", "git", "api"}}
		result := r.TopicsJSON()
		var decoded []string
		err := json.Unmarshal([]byte(result), &decoded)
		assert.NoError(t, err)
		assert.Equal(t, r.Topics, decoded)
	})
}

func TestRepository_LanguageStatsJSON(t *testing.T) {
	t.Run("nil language stats", func(t *testing.T) {
		r := &Repository{LanguageStats: nil}
		result := r.LanguageStatsJSON()
		assert.Equal(t, "null", result)
	})

	t.Run("empty language stats", func(t *testing.T) {
		r := &Repository{LanguageStats: map[string]float64{}}
		result := r.LanguageStatsJSON()
		assert.Equal(t, "{}", result)
	})

	t.Run("with language stats", func(t *testing.T) {
		r := &Repository{LanguageStats: map[string]float64{"Go": 80.5, "Python": 19.5}}
		result := r.LanguageStatsJSON()
		var decoded map[string]float64
		err := json.Unmarshal([]byte(result), &decoded)
		assert.NoError(t, err)
		assert.Equal(t, r.LanguageStats, decoded)
	})
}

func TestPost_TierIDsJSON(t *testing.T) {
	t.Run("nil tier IDs", func(t *testing.T) {
		p := &Post{TierIDs: nil}
		result := p.TierIDsJSON()
		assert.Equal(t, "null", result)
	})

	t.Run("empty tier IDs", func(t *testing.T) {
		p := &Post{TierIDs: []string{}}
		result := p.TierIDsJSON()
		assert.Equal(t, "[]", result)
	})

	t.Run("with tier IDs", func(t *testing.T) {
		p := &Post{TierIDs: []string{"tier1", "tier2", "tier3"}}
		result := p.TierIDsJSON()
		var decoded []string
		err := json.Unmarshal([]byte(result), &decoded)
		assert.NoError(t, err)
		assert.Equal(t, p.TierIDs, decoded)
	})
}

func TestContentTemplate_VariablesJSON(t *testing.T) {
	t.Run("nil variables", func(t *testing.T) {
		ct := &ContentTemplate{Variables: nil}
		result := ct.VariablesJSON()
		assert.Equal(t, "null", result)
	})

	t.Run("empty variables", func(t *testing.T) {
		ct := &ContentTemplate{Variables: []string{}}
		result := ct.VariablesJSON()
		assert.Equal(t, "[]", result)
	})

	t.Run("with variables", func(t *testing.T) {
		ct := &ContentTemplate{Variables: []string{"title", "description", "language"}}
		result := ct.VariablesJSON()
		var decoded []string
		err := json.Unmarshal([]byte(result), &decoded)
		assert.NoError(t, err)
		assert.Equal(t, ct.Variables, decoded)
	})
}
