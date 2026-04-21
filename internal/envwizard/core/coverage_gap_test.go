package core_test

import (
	"os"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/envwizard/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCurrentVar_OutOfBounds(t *testing.T) {
	vars := []*core.EnvVar{
		{Name: "A", Description: "A"},
	}
	w := core.NewWizard(vars)
	w.GoToStep(0)
	assert.NotNil(t, w.CurrentVar())

	w.Step = 5
	assert.Nil(t, w.CurrentVar())

	w.Step = -1
	assert.Nil(t, w.CurrentVar())
}

func TestWizard_UncategorizedEnvVar(t *testing.T) {
	vars := []*core.EnvVar{
		{Name: "FREE", Description: "No category"},
	}
	w := core.NewWizard(vars)
	assert.NotNil(t, w.CurrentVar())
	assert.Nil(t, w.CurrentVar().Category)
}

func TestGenerator_UncategorizedVar(t *testing.T) {
	vars := []*core.EnvVar{
		{Name: "FREE", Description: "No category"},
		{Name: "PORT", Description: "Port", Category: &core.Category{ID: "server", Name: "Server", Order: 1}},
	}
	w := core.NewWizard(vars)
	w.SetValue("FREE", "value")
	w.SetValue("PORT", "8080")

	gen := core.NewGenerator(w)
	output := gen.ProduceEnvFile(false)
	assert.Contains(t, output, "FREE=value")
	assert.Contains(t, output, "PORT=8080")
}

func TestGenerator_EmptyCategory(t *testing.T) {
	vars := []*core.EnvVar{}
	w := core.NewWizard(vars)
	gen := core.NewGenerator(w)
	output := gen.ProduceEnvFile(false)
	assert.Empty(t, output)
}

func TestGenerator_VarWithNoDescription(t *testing.T) {
	vars := []*core.EnvVar{
		{Name: "BLANK", Category: &core.Category{ID: "x", Name: "X", Order: 1}},
	}
	w := core.NewWizard(vars)
	gen := core.NewGenerator(w)
	output := gen.ProduceEnvFile(false)
	assert.Contains(t, output, "BLANK=")
}

func TestProfileDir_XDGConfigHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	p := &core.Profile{Name: "xdgtest", Values: map[string]string{"A": "1"}}
	require.NoError(t, core.SaveProfile(p))
	loaded, err := core.LoadProfile("xdgtest")
	require.NoError(t, err)
	assert.Equal(t, "1", loaded.Values["A"])
	core.DeleteProfile("xdgtest")
}

func TestProfileDir_HomeFallback(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	dir := home + "/.config"
	t.Cleanup(func() { os.RemoveAll(dir + "/patreon-manager/profiles/homefallback.json") })
	p := &core.Profile{Name: "homefallback", Values: map[string]string{"B": "2"}}
	require.NoError(t, core.SaveProfile(p))
	loaded, err := core.LoadProfile("homefallback")
	require.NoError(t, err)
	assert.Equal(t, "2", loaded.Values["B"])
}

func TestProfileSave_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	_, err := core.LoadProfile("nonexistent_profile")
	assert.Error(t, err)
}

func TestCategoryOrder_NilCategory(t *testing.T) {
	vars := []*core.EnvVar{
		{Name: "A"},
		{Name: "B", Category: &core.Category{ID: "c", Name: "C", Order: 5}},
	}
	w := core.NewWizard(vars)
	gen := core.NewGenerator(w)
	output := gen.ProduceEnvFile(false)
	assert.Contains(t, output, "A=")
	assert.Contains(t, output, "B=")
}
