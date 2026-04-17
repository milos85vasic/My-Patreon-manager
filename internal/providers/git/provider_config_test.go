package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseOrgList(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"empty string", "", nil},
		{"whitespace only", "   ", nil},
		{"single org", "myorg", []string{"myorg"}},
		{"single org with spaces", "  myorg  ", []string{"myorg"}},
		{"multiple orgs", "org1,org2,org3", []string{"org1", "org2", "org3"}},
		{"multiple orgs with spaces", " org1 , org2 , org3 ", []string{"org1", "org2", "org3"}},
		{"wildcard", "*", []string{"*"}},
		{"wildcard with spaces", " * ", []string{"*"}},
		{"trailing commas", "org1,org2,", []string{"org1", "org2"}},
		{"leading commas", ",org1,org2", []string{"org1", "org2"}},
		{"commas with spaces only", "org1,  ,org2", []string{"org1", "org2"}},
		{"special characters", "my-org,my_org", []string{"my-org", "my_org"}},
		{"single element with trailing comma", "org1,", []string{"org1"}},
		{"just commas", ",,,", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseOrgList(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestShouldScanAllOrgs(t *testing.T) {
	tests := []struct {
		name     string
		orgs     []string
		expected bool
	}{
		{"nil slice", nil, false},
		{"empty slice", []string{}, false},
		{"wildcard", []string{"*"}, true},
		{"single org", []string{"myorg"}, false},
		{"multiple orgs", []string{"org1", "org2"}, false},
		{"wildcard with others", []string{"*", "org1"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShouldScanAllOrgs(tt.orgs)
			assert.Equal(t, tt.expected, result)
		})
	}
}
