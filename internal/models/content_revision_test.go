package models_test

import (
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

func TestIsLegalRevisionStatusTransition(t *testing.T) {
	cases := []struct {
		from, to string
		want     bool
	}{
		// pending_review → legal targets
		{"pending_review", "approved", true},
		{"pending_review", "rejected", true},
		{"pending_review", "superseded", true},
		// pending_review → illegal (self, unknown)
		{"pending_review", "pending_review", false},

		// approved → only superseded
		{"approved", "superseded", true},
		{"approved", "pending_review", false},
		{"approved", "rejected", false},
		{"approved", "approved", false},

		// rejected is terminal
		{"rejected", "pending_review", false},
		{"rejected", "approved", false},
		{"rejected", "superseded", false},
		{"rejected", "rejected", false},

		// superseded is terminal
		{"superseded", "pending_review", false},
		{"superseded", "approved", false},
		{"superseded", "rejected", false},

		// unknown source state
		{"", "approved", false},
	}
	for _, c := range cases {
		got := models.IsLegalRevisionStatusTransition(c.from, c.to)
		if got != c.want {
			t.Errorf("IsLegalRevisionStatusTransition(%q,%q)=%v, want %v", c.from, c.to, got, c.want)
		}
	}
}
