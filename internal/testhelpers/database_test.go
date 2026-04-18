package testhelpers_test

import (
	"context"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/testhelpers"
)

func TestOpenMigratedSQLite_ReturnsMigratedDB(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	for _, table := range []string{"repositories", "sync_states", "generated_contents", "illustrations"} {
		var n int
		err := db.DB().QueryRowContext(context.Background(),
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&n)
		if err != nil {
			t.Fatalf("query for %s: %v", table, err)
		}
		if n != 1 {
			t.Fatalf("expected table %s to exist, got count=%d", table, n)
		}
	}
}

func TestOpenMigratedSQLite_ClosesOnTestCleanup(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	if err := db.DB().Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
}
