package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
)

type CheckpointManager struct {
	db             database.Database
	checkpointFile string
}

type Checkpoint struct {
	CompletedRepoIDs []string `json:"completed_repo_ids"`
	FailedRepoIDs    []string `json:"failed_repo_ids"`
	CurrentRepoID    string   `json:"current_repo_id"`
	StartedAt        string   `json:"started_at"`
	ResumeFrom       int      `json:"resume_from"`
}

func NewCheckpointManager(db database.Database) *CheckpointManager {
	return &CheckpointManager{
		db:             db,
		checkpointFile: filepath.Join(os.TempDir(), "patreon-manager-checkpoint.json"),
	}
}

func (cm *CheckpointManager) SaveCheckpoint(state Checkpoint) error {
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal checkpoint: %w", err)
	}
	return os.WriteFile(cm.checkpointFile, data, 0644)
}

func (cm *CheckpointManager) LoadCheckpoint() (*Checkpoint, error) {
	data, err := os.ReadFile(cm.checkpointFile)
	if err != nil {
		if os.IsNotExist(err) {
			return &Checkpoint{}, nil
		}
		return nil, fmt.Errorf("read checkpoint: %w", err)
	}
	var cp Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, fmt.Errorf("unmarshal checkpoint: %w", err)
	}
	return &cp, nil
}

func (cm *CheckpointManager) ClearCheckpoint() error {
	if err := os.Remove(cm.checkpointFile); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (cm *CheckpointManager) SaveDBCheckpoint(ctx context.Context, repoID, checkpoint string) error {
	stateStore := cm.db.SyncStates()
	existing, err := stateStore.GetByRepositoryID(ctx, repoID)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("sync state not found for repo %s", repoID)
	}
	checkpointJSON, _ := json.Marshal(checkpoint)
	return stateStore.UpdateCheckpoint(ctx, repoID, string(checkpointJSON))
}

func (cm *CheckpointManager) LoadDBCheckpoint(ctx context.Context, repoID string) (string, error) {
	stateStore := cm.db.SyncStates()
	existing, err := stateStore.GetByRepositoryID(ctx, repoID)
	if err != nil {
		return "", err
	}
	if existing == nil || existing.Checkpoint == "" || existing.Checkpoint == "{}" {
		return "", nil
	}
	var checkpoint string
	if err := json.Unmarshal([]byte(existing.Checkpoint), &checkpoint); err != nil {
		return "", err
	}
	return checkpoint, nil
}
