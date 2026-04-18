package models_test

import (
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

func TestProcessRunStatuses(t *testing.T) {
	for _, s := range []string{
		models.ProcessRunStatusRunning,
		models.ProcessRunStatusFinished,
		models.ProcessRunStatusCrashed,
		models.ProcessRunStatusAborted,
	} {
		if s == "" {
			t.Fatalf("empty status constant")
		}
	}
}
