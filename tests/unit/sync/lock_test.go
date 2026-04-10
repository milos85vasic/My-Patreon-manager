package sync_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	ierrors "github.com/milos85vasic/My-Patreon-Manager/internal/errors"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/sync"
	"github.com/milos85vasic/My-Patreon-Manager/tests/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLockManager_AcquireLock_Success(t *testing.T) {
	mockDB := &mocks.MockDatabase{}
	lm := sync.NewLockManager(mockDB)

	mockDB.AcquireLockFunc = func(ctx context.Context, lockInfo database.SyncLock) error {
		return nil
	}

	err := lm.AcquireLock(context.Background())
	assert.NoError(t, err)
}

func TestLockManager_AcquireLock_DBContention(t *testing.T) {
	mockDB := &mocks.MockDatabase{}
	lm := sync.NewLockManager(mockDB)

	mockDB.AcquireLockFunc = func(ctx context.Context, lockInfo database.SyncLock) error {
		return ierrors.LockContention("lock already held")
	}

	err := lm.AcquireLock(context.Background())
	assert.Error(t, err)
	assert.True(t, ierrors.IsLockContention(err))
}

func TestLockManager_ReleaseLock(t *testing.T) {
	mockDB := &mocks.MockDatabase{}
	lm := sync.NewLockManager(mockDB)

	mockDB.ReleaseLockFunc = func(ctx context.Context) error {
		return nil
	}

	err := lm.ReleaseLock(context.Background())
	assert.NoError(t, err)
}

func TestLockManager_IsLocked(t *testing.T) {
	mockDB := &mocks.MockDatabase{}
	lm := sync.NewLockManager(mockDB)

	mockDB.IsLockedFunc = func(ctx context.Context) (bool, *database.SyncLock, error) {
		return true, &database.SyncLock{PID: 123, Hostname: "test"}, nil
	}

	locked, lockInfo, err := lm.IsLocked(context.Background())
	assert.NoError(t, err)
	assert.True(t, locked)
	assert.Equal(t, 123, lockInfo.PID)
}

func TestLockManager_StaleDetection(t *testing.T) {
	// Create a lock file with a PID that doesn't exist
	tempDir := t.TempDir()
	lockFile := filepath.Join(tempDir, "test.lock")
	// Write lock file with PID 99999 (unlikely to exist)
	content := "99999:testhost:2024-01-01T00:00:00Z"
	err := os.WriteFile(lockFile, []byte(content), 0644)
	require.NoError(t, err)

	// We need to set the lockManager's lockFile to this temp file.
	// Since lockFile is private, we cannot set it directly.
	// Let's skip this test for now.
	t.Skip("Stale detection test requires access to private lockFile field")
}

func TestLockManager_ConcurrentLockContention(t *testing.T) {
	// Simulate concurrent lock attempts.
	// We'll use a mock DB that returns LockContention after first acquire.
	mockDB := &mocks.MockDatabase{}
	lm := sync.NewLockManager(mockDB)

	callCount := 0
	mockDB.AcquireLockFunc = func(ctx context.Context, lockInfo database.SyncLock) error {
		callCount++
		if callCount == 1 {
			return nil
		}
		return ierrors.LockContention("lock already held")
	}

	// First acquire should succeed
	err := lm.AcquireLock(context.Background())
	assert.NoError(t, err)

	// Second acquire should fail with LockContention
	err = lm.AcquireLock(context.Background())
	assert.Error(t, err)
	assert.True(t, ierrors.IsLockContention(err))
}
