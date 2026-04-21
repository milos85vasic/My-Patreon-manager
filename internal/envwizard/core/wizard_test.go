package core_test

import (
	"errors"
	"os"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/envwizard/core"
	"github.com/stretchr/testify/assert"
)

var testVars = []*core.EnvVar{
	{Name: "PORT", Description: "HTTP port", Required: true, Validation: core.ValidationPort, Default: "8080"},
	{Name: "ADMIN_KEY", Description: "Admin key", Required: true, Secret: true, CanGenerate: true},
	{Name: "REVIEWER_KEY", Description: "Reviewer key", Required: false, Secret: true},
	{Name: "HMAC_SECRET", Description: "HMAC secret", Required: true, Secret: true, CanGenerate: true},
	{Name: "DEBUG", Description: "Debug mode", Required: false, Default: "false", Validation: core.ValidationBoolean},
}

func TestNewWizard(t *testing.T) {
	w := core.NewWizard(testVars)
	assert.Equal(t, 0, w.Step)
	assert.Empty(t, w.Values)
	assert.Empty(t, w.Skipped)
	assert.Equal(t, testVars, w.Vars)
}

func TestWizard_Navigation(t *testing.T) {
	w := core.NewWizard(testVars)
	assert.Equal(t, 0, w.Step)

	ok := w.Next()
	assert.True(t, ok)
	assert.Equal(t, 1, w.Step)

	ok = w.Previous()
	assert.True(t, ok)
	assert.Equal(t, 0, w.Step)
}

func TestWizard_NextAtEnd(t *testing.T) {
	w := core.NewWizard(testVars)
	w.Step = w.TotalSteps() - 1
	ok := w.Next()
	assert.False(t, ok)
}

func TestWizard_PreviousAtStart(t *testing.T) {
	w := core.NewWizard(testVars)
	ok := w.Previous()
	assert.False(t, ok)
}

func TestWizard_GoToStep(t *testing.T) {
	w := core.NewWizard(testVars)
	ok := w.GoToStep(3)
	assert.True(t, ok)
	assert.Equal(t, 3, w.Step)

	ok = w.GoToStep(-1)
	assert.False(t, ok)

	ok = w.GoToStep(99999)
	assert.False(t, ok)
}

func TestWizard_SetGetValue(t *testing.T) {
	w := core.NewWizard(testVars)
	w.SetValue("PORT", "3000")
	assert.Equal(t, "3000", w.GetValue("PORT"))
	assert.True(t, w.IsSet("PORT"))
}

func TestWizard_Skip(t *testing.T) {
	w := core.NewWizard(testVars)
	w.Skip("REVIEWER_KEY")
	assert.True(t, w.IsSkipped("REVIEWER_KEY"))
	assert.False(t, w.IsSet("REVIEWER_KEY"))
}

func TestWizard_SetValueClearsSkip(t *testing.T) {
	w := core.NewWizard(testVars)
	w.Skip("PORT")
	w.SetValue("PORT", "8080")
	assert.False(t, w.IsSkipped("PORT"))
	assert.Equal(t, "8080", w.GetValue("PORT"))
}

func TestWizard_Errors(t *testing.T) {
	w := core.NewWizard(testVars)
	w.SetError("PORT", errors.New("invalid port"))
	assert.True(t, w.HasErrors())
	assert.Error(t, w.GetError("PORT"))
}

func TestWizard_SetValueClearsError(t *testing.T) {
	w := core.NewWizard(testVars)
	w.SetError("PORT", errors.New("invalid"))
	w.SetValue("PORT", "8080")
	assert.NoError(t, w.GetError("PORT"))
}

func TestWizard_MissingRequired(t *testing.T) {
	w := core.NewWizard(testVars)
	missing := w.MissingRequired()
	assert.Equal(t, 3, len(missing)) // PORT, ADMIN_KEY, HMAC_SECRET
}

func TestWizard_MissingRequired_NoneAfterSetting(t *testing.T) {
	w := core.NewWizard(testVars)
	w.SetValue("PORT", "8080")
	w.SetValue("ADMIN_KEY", "key123")
	w.SetValue("HMAC_SECRET", "secret")
	missing := w.MissingRequired()
	assert.Empty(t, missing)
}

func TestWizard_Progress(t *testing.T) {
	w := core.NewWizard(testVars)
	completed, total := w.Progress()
	assert.Equal(t, 0, completed)
	assert.Equal(t, 5, total)

	w.SetValue("PORT", "8080")
	completed, total = w.Progress()
	assert.Equal(t, 1, completed)
}

func TestWizard_CurrentVar(t *testing.T) {
	w := core.NewWizard(testVars)
	v := w.CurrentVar()
	assert.NotNil(t, v)
	assert.Equal(t, "PORT", v.Name)
}

func TestNewWizardFromEnvFile(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test*.env")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	tmpFile.WriteString("PORT=9000\nADMIN_KEY=test123\n")
	tmpFile.Close()

	w, err := core.NewWizardFromEnvFile(testVars, tmpFile.Name())
	assert.NoError(t, err)
	assert.Equal(t, "9000", w.GetValue("PORT"))
	assert.Equal(t, "test123", w.GetValue("ADMIN_KEY"))
}

func TestNewWizardFromEnvFile_NotFound(t *testing.T) {
	_, err := core.NewWizardFromEnvFile(testVars, "/nonexistent/.env")
	assert.Error(t, err)
}
