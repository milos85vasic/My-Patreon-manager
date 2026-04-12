package sync_test

import (
	"context"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	ierrors "github.com/milos85vasic/My-Patreon-Manager/internal/errors"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/sync"
	"github.com/milos85vasic/My-Patreon-Manager/tests/mocks"
	"github.com/stretchr/testify/assert"
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
	// Test stale-lock detection via the DB path: when the database reports
	// a lock held by a process that no longer exists (stale), the LockManager
	// should still report the DB state. The file-based stale detection is
	// tested within the internal package using ExportedSetLockFile.
	mockDB := &mocks.MockDatabase{}
	lm := sync.NewLockManager(mockDB)

	// Simulate a DB lock held by a dead PID (99999 is unlikely to exist)
	mockDB.IsLockedFunc = func(ctx context.Context) (bool, *database.SyncLock, error) {
		return true, &database.SyncLock{PID: 99999, Hostname: "stalehost"}, nil
	}

	locked, lockInfo, err := lm.IsLocked(context.Background())
	assert.NoError(t, err)
	// The DB says locked, so IsLocked reports true (file-based check may
	// independently report not-locked, but the DB path takes precedence
	// when the file lock doesn't exist).
	assert.True(t, locked)
	assert.Equal(t, 99999, lockInfo.PID)
	assert.Equal(t, "stalehost", lockInfo.Hostname)
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
