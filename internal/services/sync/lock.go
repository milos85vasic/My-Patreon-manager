package sync

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/errors"
	"github.com/milos85vasic/My-Patreon-Manager/internal/utils"
)

type LockManager struct {
	db       database.Database
	lockFile string
	hostname string
	mu       sync.Mutex
}

func NewLockManager(db database.Database) *LockManager {
	hostname, _ := os.Hostname()
	return &LockManager{
		db:       db,
		lockFile: "/tmp/patreon-manager-sync.lock",
		hostname: hostname,
	}
}

func (lm *LockManager) AcquireLock(ctx context.Context) error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	pid := os.Getpid()
	now := time.Now()
	expires := now.Add(24 * time.Hour)

	lockInfo := database.SyncLock{
		ID:        utils.NewUUID(),
		PID:       pid,
		Hostname:  lm.hostname,
		StartedAt: now,
		ExpiresAt: expires,
	}

	if err := lm.acquireFileLock(pid); err != nil {
		return err
	}

	if err := lm.db.AcquireLock(ctx, lockInfo); err != nil {
		lm.releaseFileLock()
		return errors.LockContention(fmt.Sprintf("DB lock failed: %v", err))
	}

	return nil
}

func (lm *LockManager) ReleaseLock(ctx context.Context) error {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	lm.releaseFileLock()
	return lm.db.ReleaseLock(ctx)
}

func (lm *LockManager) IsLocked(ctx context.Context) (bool, *database.SyncLock, error) {
	locked, err := lm.isFileLocked()
	if err != nil {
		return false, nil, err
	}
	if locked {
		lock := &database.SyncLock{PID: -1, Hostname: "unknown"}
		return true, lock, nil
	}
	return lm.db.IsLocked(ctx)
}

func (lm *LockManager) acquireFileLock(pid int) error {
	content := fmt.Sprintf("%d:%s:%s", pid, lm.hostname, time.Now().Format(time.RFC3339))
	return os.WriteFile(lm.lockFile, []byte(content), 0644)
}

func (lm *LockManager) releaseFileLock() {
	os.Remove(lm.lockFile)
}

func (lm *LockManager) isFileLocked() (bool, error) {
	content, err := os.ReadFile(lm.lockFile)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	line := strings.TrimSpace(string(content))
	if line == "" {
		return false, nil
	}

	fields := strings.SplitN(line, ":", 3)
	if len(fields) < 1 {
		return false, nil
	}

	lockedPID, err := strconv.Atoi(fields[0])
	if err != nil {
		return false, nil
	}

	if lockedPID == os.Getpid() {
		return true, nil
	}

	process, err := os.FindProcess(lockedPID)
	if err != nil {
		return false, nil
	}

	err = process.Signal(syscall.Signal(0))
	if err == nil {
		return true, nil
	}

	if pe, ok := err.(*os.PathError); ok && pe.Err == syscall.ESRCH {
		os.Remove(lm.lockFile)
		return false, nil
	}

	return true, nil
}
