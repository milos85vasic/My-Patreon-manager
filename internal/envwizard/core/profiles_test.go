package core_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/envwizard/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestProfileDir(t *testing.T) string {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	return dir
}

func TestSaveAndLoadProfile(t *testing.T) {
	setupTestProfileDir(t)

	p := &core.Profile{
		Name:   "development",
		Values: map[string]string{"PORT": "8080", "ADMIN_KEY": "secret"},
	}

	err := core.SaveProfile(p)
	require.NoError(t, err)

	loaded, err := core.LoadProfile("development")
	require.NoError(t, err)
	assert.Equal(t, "development", loaded.Name)
	assert.Equal(t, "8080", loaded.Values["PORT"])
	assert.Equal(t, "secret", loaded.Values["ADMIN_KEY"])
}

func TestLoadProfile_NotFound(t *testing.T) {
	setupTestProfileDir(t)

	_, err := core.LoadProfile("nonexistent")
	assert.Error(t, err)
	assert.True(t, os.IsNotExist(err))
}

func TestListProfiles(t *testing.T) {
	setupTestProfileDir(t)

	p1 := &core.Profile{Name: "alpha", Values: map[string]string{"PORT": "3000"}}
	p2 := &core.Profile{Name: "beta", Values: map[string]string{"PORT": "4000"}}

	require.NoError(t, core.SaveProfile(p1))
	require.NoError(t, core.SaveProfile(p2))

	names, err := core.ListProfiles()
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha", "beta"}, names)
}

func TestListProfiles_Empty(t *testing.T) {
	setupTestProfileDir(t)

	names, err := core.ListProfiles()
	require.NoError(t, err)
	assert.Empty(t, names)
}

func TestDeleteProfile(t *testing.T) {
	setupTestProfileDir(t)

	p := &core.Profile{Name: "to-delete", Values: map[string]string{"PORT": "8080"}}
	require.NoError(t, core.SaveProfile(p))

	err := core.DeleteProfile("to-delete")
	require.NoError(t, err)

	_, err = core.LoadProfile("to-delete")
	assert.Error(t, err)
}

func TestDeleteProfile_NotFound(t *testing.T) {
	setupTestProfileDir(t)

	err := core.DeleteProfile("nonexistent")
	assert.Error(t, err)
}

func TestProfileFilePermissions(t *testing.T) {
	dir := setupTestProfileDir(t)

	p := &core.Profile{Name: "secure", Values: map[string]string{"SECRET": "value"}}
	require.NoError(t, core.SaveProfile(p))

	path := filepath.Join(dir, "patreon-manager", "profiles", "secure.json")
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}
