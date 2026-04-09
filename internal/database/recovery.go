package database

import (
	"context"
	"fmt"
	"log/slog"
	"os"
)

func RecoverSQLite(ctx context.Context, dbPath string, logger *slog.Logger) error {
	sqlite := NewSQLiteDB(dbPath)
	err := sqlite.Connect(ctx, dbPath)
	if err != nil {
		if logger != nil {
			logger.Error("sqlite integrity check failed, attempting recovery", slog.String("error", err.Error()))
		}
		backupPath := dbPath + ".corrupted"
		if err := os.Rename(dbPath, backupPath); err != nil {
			return fmt.Errorf("backup corrupted db: %w", err)
		}
		if logger != nil {
			logger.Warn("corrupted database backed up", slog.String("backup", backupPath))
		}
		sqlite = NewSQLiteDB(dbPath)
		if err := sqlite.Connect(ctx, dbPath); err != nil {
			return fmt.Errorf("reinitialize db: %w", err)
		}
		if err := sqlite.Migrate(ctx); err != nil {
			return fmt.Errorf("re-run migrations: %w", err)
		}
	}
	return nil
}
