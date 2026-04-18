package models_test

import (
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

func TestUnmatchedPatreonPost_ZeroValue(t *testing.T) {
	var p models.UnmatchedPatreonPost
	if p.ID != "" || p.PatreonPostID != "" {
		t.Fatalf("unexpected zero value: %+v", p)
	}
}
