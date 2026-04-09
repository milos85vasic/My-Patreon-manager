package sync_test

import (
	"encoding/json"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/services/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestCheckpointManager(t *testing.T) *sync.CheckpointManager {
	t.Helper()
	return sync.NewCheckpointManagerWithFile(t.TempDir())
}

func TestCheckpointManager_SaveAndLoad(t *testing.T) {
	cm := newTestCheckpointManager(t)

	state := sync.Checkpoint{
		CompletedRepoIDs: []string{"repo1", "repo2"},
		FailedRepoIDs:    []string{"repo3"},
		CurrentRepoID:    "repo4",
		StartedAt:        "2024-01-01T00:00:00Z",
		ResumeFrom:       2,
	}

	err := cm.SaveCheckpoint(state)
	require.NoError(t, err)

	loaded, err := cm.LoadCheckpoint()
	require.NoError(t, err)
	assert.Equal(t, state.CompletedRepoIDs, loaded.CompletedRepoIDs)
	assert.Equal(t, state.CurrentRepoID, loaded.CurrentRepoID)
	assert.Equal(t, state.ResumeFrom, loaded.ResumeFrom)
}

func TestCheckpointManager_LoadNonExistent(t *testing.T) {
	cm := newTestCheckpointManager(t)
	cp, err := cm.LoadCheckpoint()
	assert.NoError(t, err)
	assert.NotNil(t, cp)
	assert.Empty(t, cp.CompletedRepoIDs)
}

func TestCheckpointManager_Clear(t *testing.T) {
	cm := newTestCheckpointManager(t)
	err := cm.SaveCheckpoint(sync.Checkpoint{CompletedRepoIDs: []string{"repo1"}})
	require.NoError(t, err)
	err = cm.ClearCheckpoint()
	assert.NoError(t, err)
}

func TestCheckpoint_JSONRoundTrip(t *testing.T) {
	cp := sync.Checkpoint{
		CompletedRepoIDs: []string{"a", "b"},
		FailedRepoIDs:    []string{"c"},
		CurrentRepoID:    "d",
		StartedAt:        "2024-01-01T00:00:00Z",
		ResumeFrom:       1,
	}
	data, err := json.Marshal(cp)
	require.NoError(t, err)
	var decoded sync.Checkpoint
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, cp, decoded)
}
