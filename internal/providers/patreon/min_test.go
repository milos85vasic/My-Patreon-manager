package patreon

import (
	"testing"
)

func TestMin(t *testing.T) {
	oauth := NewOAuth2Manager("a", "b", "c", "d")
	if oauth == nil {
		t.Fatal("oauth is nil")
	}
	client := NewClient(oauth, "campaign")
	if client == nil {
		t.Fatal("client is nil")
	}
}
