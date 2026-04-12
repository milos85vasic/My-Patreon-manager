package database_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecoverSQLite_Success(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create a valid SQLite database by connecting and migrating
	db := database.NewSQLiteDB(dbPath)
	ctx := context.Background()
	err := db.Connect(ctx, dbPath)
	require.NoError(t, err)
	err = db.Migrate(ctx)
	require.NoError(t, err)
	db.Close()

	err = database.RecoverSQLite(ctx, dbPath, nil) // nil logger
	assert.NoError(t, err)
	// Database should still exist
	_, err = os.Stat(dbPath)
	assert.NoError(t, err)
}

func TestRecoverSQLite_CorruptedRecovery(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create a corrupted SQLite file (just random bytes)
	err := os.WriteFile(dbPath, []byte("not a valid sqlite file"), 0644)
	require.NoError(t, err)

	ctx := context.Background()
	err = database.RecoverSQLite(ctx, dbPath, nil)
	assert.NoError(t, err)

	// Backup file should exist
	backupPath := dbPath + ".corrupted"
	_, err = os.Stat(backupPath)
	assert.NoError(t, err)

	// New database should be created and migrated (valid SQLite)
	// Verify by connecting
	db := database.NewSQLiteDB(dbPath)
	err = db.Connect(ctx, dbPath)
	assert.NoError(t, err)
	db.Close()
}

func TestRecoverSQLite_CorruptedBackupFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based test not reliable on Windows")
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create corrupted file
	err := os.WriteFile(dbPath, []byte("corrupt"), 0644)
	require.NoError(t, err)

	// Make the directory read-only so rename fails (simulate backup failure)
	err = os.Chmod(tmpDir, 0555)
	require.NoError(t, err)
	// Restore permissions so t.TempDir cleanup succeeds
	defer os.Chmod(tmpDir, 0755)

	ctx := context.Background()
	err = database.RecoverSQLite(ctx, dbPath, nil)
	assert.Error(t, err, "RecoverSQLite should fail when backup rename is impossible")
}
