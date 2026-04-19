package testhelpers

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
)

// fakeReporter implements fatalReporter without aborting the host test
// process. Fatalf just records the message so the caller can assert on it.
type fakeReporter struct {
	helperCalled bool
	fatalMsg     string
	cleanups     []func()
}

func (f *fakeReporter) Helper() { f.helperCalled = true }
func (f *fakeReporter) Fatalf(format string, args ...any) {
	// Avoid pulling fmt just for Sprintf; a simple concat is enough for
	// test assertions.
	parts := []string{format}
	for _, a := range args {
		if err, ok := a.(error); ok {
			parts = append(parts, err.Error())
		}
	}
	f.fatalMsg = strings.Join(parts, " ")
}
func (f *fakeReporter) Cleanup(fn func()) { f.cleanups = append(f.cleanups, fn) }

// TestOpenMigratedSQLite_ConnectFailure drives the Connect error branch
// that real SQLite ":memory:" can't trigger.
func TestOpenMigratedSQLite_ConnectFailure(t *testing.T) {
	origConnect := sqliteConnect
	defer func() { sqliteConnect = origConnect }()
	sqliteConnect = func(_ *database.SQLiteDB, _ context.Context, _ string) error {
		return errors.New("connect boom")
	}

	r := &fakeReporter{}
	got := openMigratedSQLite(r)
	if got != nil {
		t.Fatalf("want nil return on connect failure, got %v", got)
	}
	if !r.helperCalled {
		t.Fatal("Helper() must be called")
	}
	if !strings.Contains(r.fatalMsg, "connect sqlite") {
		t.Fatalf("want connect sqlite in fatal, got %q", r.fatalMsg)
	}
	if !strings.Contains(r.fatalMsg, "connect boom") {
		t.Fatalf("want boom in fatal, got %q", r.fatalMsg)
	}
}

// TestOpenMigratedSQLite_MigrateFailure drives the Migrate error branch
// and verifies that Close is invoked before the fatal report.
func TestOpenMigratedSQLite_MigrateFailure(t *testing.T) {
	origMigrate := sqliteMigrate
	origClose := sqliteCloseFn
	defer func() {
		sqliteMigrate = origMigrate
		sqliteCloseFn = origClose
	}()
	sqliteMigrate = func(_ *database.SQLiteDB, _ context.Context) error {
		return errors.New("migrate boom")
	}
	closeCalled := false
	sqliteCloseFn = func(_ *database.SQLiteDB) error {
		closeCalled = true
		return nil
	}

	r := &fakeReporter{}
	got := openMigratedSQLite(r)
	if got != nil {
		t.Fatalf("want nil return on migrate failure, got %v", got)
	}
	if !closeCalled {
		t.Fatal("Close must be invoked before Fatalf")
	}
	if !strings.Contains(r.fatalMsg, "migrate sqlite") {
		t.Fatalf("want migrate sqlite in fatal, got %q", r.fatalMsg)
	}
}

// TestGoleakIgnores_ReturnsExpectedOptions verifies GoleakIgnores returns
// a non-empty slice; the exact contents are implementation details
// covered by the goleak library's own tests.
func TestGoleakIgnores_ReturnsExpectedOptions(t *testing.T) {
	opts := GoleakIgnores()
	if len(opts) == 0 {
		t.Fatal("GoleakIgnores must return at least one option")
	}
}
