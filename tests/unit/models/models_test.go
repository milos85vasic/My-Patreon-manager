package models

import (
	"encoding/json"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestRepository_TopicsJSON(t *testing.T) {
	tests := []struct {
		name     string
		topics   []string
		expected string
	}{
		{
			name:     "empty slice",
			topics:   []string{},
			expected: "[]",
		},
		{
			name:     "single topic",
			topics:   []string{"go"},
			expected: `["go"]`,
		},
		{
			name:     "multiple topics",
			topics:   []string{"go", "git", "api"},
			expected: `["go","git","api"]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &models.Repository{Topics: tt.topics}
			result := r.TopicsJSON()
			assert.Equal(t, tt.expected, result)
			// Ensure valid JSON
			var decoded []string
			err := json.Unmarshal([]byte(result), &decoded)
			assert.NoError(t, err)
			assert.Equal(t, tt.topics, decoded)
		})
	}
}

func TestRepository_LanguageStatsJSON(t *testing.T) {
	tests := []struct {
		name   string
		stats  map[string]float64
		expect string
	}{
		{
			name:   "empty map",
			stats:  map[string]float64{},
			expect: "{}",
		},
		{
			name:   "single language",
			stats:  map[string]float64{"Go": 90.5},
			expect: `{"Go":90.5}`,
		},
		{
			name: "multiple languages",
			stats: map[string]float64{
				"Go":         80.2,
				"JavaScript": 15.7,
				"CSS":        4.1,
			},
			expect: `{"CSS":4.1,"Go":80.2,"JavaScript":15.7}`, // sorted keys
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &models.Repository{LanguageStats: tt.stats}
			result := r.LanguageStatsJSON()
			// JSON keys are unordered; parse and compare maps
			var decoded map[string]float64
			err := json.Unmarshal([]byte(result), &decoded)
			assert.NoError(t, err)
			assert.Equal(t, tt.stats, decoded)
		})
	}
}

func TestContentTemplate_VariablesJSON(t *testing.T) {
	tests := []struct {
		name     string
		vars     []string
		expected string
	}{
		{
			name:     "empty slice",
			vars:     []string{},
			expected: "[]",
		},
		{
			name:     "single variable",
			vars:     []string{"repo_name"},
			expected: `["repo_name"]`,
		},
		{
			name:     "multiple variables",
			vars:     []string{"repo_name", "owner", "description"},
			expected: `["repo_name","owner","description"]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ct := &models.ContentTemplate{Variables: tt.vars}
			result := ct.VariablesJSON()
			assert.Equal(t, tt.expected, result)
			var decoded []string
			err := json.Unmarshal([]byte(result), &decoded)
			assert.NoError(t, err)
			assert.Equal(t, tt.vars, decoded)
		})
	}
}

func TestPost_TierIDsJSON(t *testing.T) {
	tests := []struct {
		name    string
		tierIDs []string
		expect  string
	}{
		{
			name:    "empty slice",
			tierIDs: []string{},
			expect:  "[]",
		},
		{
			name:    "single tier",
			tierIDs: []string{"tier_123"},
			expect:  `["tier_123"]`,
		},
		{
			name:    "multiple tiers",
			tierIDs: []string{"tier_123", "tier_456", "tier_789"},
			expect:  `["tier_123","tier_456","tier_789"]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &models.Post{TierIDs: tt.tierIDs}
			result := p.TierIDsJSON()
			assert.Equal(t, tt.expect, result)
			var decoded []string
			err := json.Unmarshal([]byte(result), &decoded)
			assert.NoError(t, err)
			assert.Equal(t, tt.tierIDs, decoded)
		})
	}
}

// Additional tests for struct defaults (optional but ensures coverage)
func TestRepository_DefaultValues(t *testing.T) {
	r := &models.Repository{}
	assert.Empty(t, r.ID)
	assert.Empty(t, r.Service)
	assert.Empty(t, r.Owner)
	assert.Empty(t, r.Name)
	assert.Empty(t, r.URL)
	assert.Empty(t, r.HTTPSURL)
	assert.Empty(t, r.Description)
	assert.Empty(t, r.READMEContent)
	assert.Empty(t, r.READMEFormat)
	assert.Nil(t, r.Topics)
	assert.Empty(t, r.PrimaryLanguage)
	assert.Nil(t, r.LanguageStats)
	assert.Equal(t, 0, r.Stars)
	assert.Equal(t, 0, r.Forks)
	assert.Empty(t, r.LastCommitSHA)
	assert.True(t, r.LastCommitAt.IsZero())
	assert.False(t, r.IsArchived)
	assert.True(t, r.CreatedAt.IsZero())
	assert.True(t, r.UpdatedAt.IsZero())
}

func TestSyncState_DefaultValues(t *testing.T) {
	ss := &models.SyncState{}
	assert.Empty(t, ss.ID)
	assert.Empty(t, ss.RepositoryID)
	assert.Empty(t, ss.PatreonPostID)
	assert.True(t, ss.LastSyncAt.IsZero())
	assert.Empty(t, ss.LastCommitSHA)
	assert.Empty(t, ss.LastContentHash)
	assert.Empty(t, ss.Status)
	assert.Empty(t, ss.LastFailureReason)
	assert.True(t, ss.GracePeriodUntil.IsZero())
	assert.Empty(t, ss.Checkpoint)
	assert.True(t, ss.CreatedAt.IsZero())
	assert.True(t, ss.UpdatedAt.IsZero())
}

func TestGeneratedContent_DefaultValues(t *testing.T) {
	gc := &models.GeneratedContent{}
	assert.Empty(t, gc.ID)
	assert.Empty(t, gc.RepositoryID)
	assert.Empty(t, gc.ContentType)
	assert.Empty(t, gc.Format)
	assert.Empty(t, gc.Title)
	assert.Empty(t, gc.Body)
	assert.Equal(t, 0.0, gc.QualityScore)
	assert.Empty(t, gc.ModelUsed)
	assert.Empty(t, gc.PromptTemplate)
	assert.Equal(t, 0, gc.TokenCount)
	assert.Equal(t, 0, gc.GenerationAttempts)
	assert.False(t, gc.PassedQualityGate)
	assert.True(t, gc.CreatedAt.IsZero())
}

func TestCampaign_DefaultValues(t *testing.T) {
	c := &models.Campaign{}
	assert.Empty(t, c.ID)
	assert.Empty(t, c.Name)
	assert.Empty(t, c.Summary)
	assert.Empty(t, c.CreatorName)
	assert.Equal(t, 0, c.PatronCount)
	assert.True(t, c.CreatedAt.IsZero())
	assert.True(t, c.UpdatedAt.IsZero())
}

func TestTier_DefaultValues(t *testing.T) {
	ti := &models.Tier{}
	assert.Empty(t, ti.ID)
	assert.Empty(t, ti.CampaignID)
	assert.Empty(t, ti.Title)
	assert.Empty(t, ti.Description)
	assert.Equal(t, 0, ti.AmountCents)
	assert.Equal(t, 0, ti.PatronCount)
	assert.True(t, ti.CreatedAt.IsZero())
	assert.True(t, ti.UpdatedAt.IsZero())
}

func TestAuditEntry_DefaultValues(t *testing.T) {
	ae := &models.AuditEntry{}
	assert.Empty(t, ae.ID)
	assert.Empty(t, ae.RepositoryID)
	assert.Empty(t, ae.EventType)
	assert.Empty(t, ae.SourceState)
	assert.Empty(t, ae.GenerationParams)
	assert.Empty(t, ae.PublicationMeta)
	assert.Empty(t, ae.Actor)
	assert.Empty(t, ae.Outcome)
	assert.Empty(t, ae.ErrorMessage)
	assert.True(t, ae.Timestamp.IsZero())
}

func TestSourceState_DefaultValues(t *testing.T) {
	ss := &models.SourceState{}
	assert.Empty(t, ss.Service)
	assert.Empty(t, ss.Owner)
	assert.Empty(t, ss.Name)
	assert.Empty(t, ss.LastCommitSHA)
	assert.False(t, ss.IsArchived)
	assert.Empty(t, ss.Description)
	assert.Equal(t, 0, ss.Stars)
}

func TestGenerationParams_DefaultValues(t *testing.T) {
	gp := &models.GenerationParams{}
	assert.Empty(t, gp.ModelID)
	assert.Empty(t, gp.PromptTemplate)
	assert.Empty(t, gp.ContentType)
	assert.Equal(t, 0, gp.TokenCount)
	assert.Equal(t, 0, gp.Attempts)
}

func TestPublicationMeta_DefaultValues(t *testing.T) {
	pm := &models.PublicationMeta{}
	assert.Empty(t, pm.PatreonPostID)
	assert.Nil(t, pm.TierIDs)
	assert.Empty(t, pm.Format)
	assert.Empty(t, pm.Status)
}
