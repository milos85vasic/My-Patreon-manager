package database

import (
	"context"
	"testing"
)

func TestNewDatabase_SQLite(t *testing.T) {
	db := NewDatabase("sqlite", ":memory:")
	if db == nil {
		t.Fatal("expected non-nil database")
	}
}

func TestNewDatabase_Postgres(t *testing.T) {
	db := NewDatabase("postgres", "mock://localhost")
	if db == nil {
		t.Fatal("expected non-nil database")
	}
}

func TestNewDatabase_PostgresAliases(t *testing.T) {
	for _, driver := range []string{"pg", "postgresql"} {
		db := NewDatabase(driver, "mock://localhost")
		if db == nil {
			t.Fatalf("expected non-nil database for driver %q", driver)
		}
	}
}

func TestNewDatabase_Default(t *testing.T) {
	db := NewDatabase("unknown", ":memory:")
	if db == nil {
		t.Fatal("expected non-nil database for unknown driver (defaults to sqlite)")
	}
}

func TestSQLiteDB_ConnectAndMigrate(t *testing.T) {
	db := NewSQLiteDB(":memory:")
	ctx := context.Background()

	err := db.Connect(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	err = db.Migrate(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Verify stores are non-nil
	if db.Repositories() == nil {
		t.Error("expected non-nil Repositories")
	}
	if db.SyncStates() == nil {
		t.Error("expected non-nil SyncStates")
	}
	if db.MirrorMaps() == nil {
		t.Error("expected non-nil MirrorMaps")
	}
	if db.GeneratedContents() == nil {
		t.Error("expected non-nil GeneratedContents")
	}
	if db.ContentTemplates() == nil {
		t.Error("expected non-nil ContentTemplates")
	}
	if db.Posts() == nil {
		t.Error("expected non-nil Posts")
	}
	if db.AuditEntries() == nil {
		t.Error("expected non-nil AuditEntries")
	}
	if db.DB() == nil {
		t.Error("expected non-nil DB()")
	}
}

func TestSQLiteDB_ConnectWithDSN(t *testing.T) {
	db := NewSQLiteDB("")
	ctx := context.Background()
	err := db.Connect(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
}

func TestSQLiteDB_Close_Nil(t *testing.T) {
	db := NewSQLiteDB(":memory:")
	// Close without Connect should not error
	err := db.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteDB_BeginTx(t *testing.T) {
	db := NewSQLiteDB(":memory:")
	ctx := context.Background()
	if err := db.Connect(ctx, ""); err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	tx, err := db.BeginTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if tx == nil {
		t.Fatal("expected non-nil tx")
	}
	tx.Rollback()
}

func TestSQLiteDB_LockRelease(t *testing.T) {
	db := NewSQLiteDB(":memory:")
	ctx := context.Background()
	if err := db.Connect(ctx, ""); err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	lock := SyncLock{
		ID:       "lock1",
		PID:      12345,
		Hostname: "testhost",
	}
	err := db.AcquireLock(ctx, lock)
	if err != nil {
		t.Fatal(err)
	}

	locked, lockInfo, err := db.IsLocked(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !locked {
		t.Error("expected locked")
	}
	if lockInfo == nil {
		t.Error("expected non-nil lock info")
	}

	err = db.ReleaseLock(ctx)
	if err != nil {
		t.Fatal(err)
	}

	locked, _, err = db.IsLocked(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if locked {
		t.Error("expected not locked after release")
	}
}

// RunMigrations is tested implicitly through Migrate; the embedded SQL files
// use Postgres-specific syntax (::timestamp) that SQLite cannot parse, so we
// skip a direct RunMigrations call in in-memory SQLite tests.

func TestRecoverSQLite_HealthyDB(t *testing.T) {
	// Test with a healthy in-memory database (should succeed without recovery)
	dir := t.TempDir()
	dbPath := dir + "/test.db"
	ctx := context.Background()

	// Create a healthy db first
	db := NewSQLiteDB(dbPath)
	if err := db.Connect(ctx, dbPath); err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	db.Close()

	// Now recover
	err := RecoverSQLite(ctx, dbPath, nil)
	if err != nil {
		t.Fatal(err)
	}
}
