package cli_test

import (
	"bufio"
	"os"
	"strings"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/envwizard/core"
	envcli "github.com/milos85vasic/My-Patreon-Manager/cmd/envwizard/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var cliTestVars = []*core.EnvVar{
	{Name: "PORT", Description: "HTTP port", Required: true, Validation: core.ValidationPort, Default: "8080"},
	{Name: "ADMIN_KEY", Description: "Admin key", Required: true, Secret: true, CanGenerate: true},
	{Name: "REVIEWER_KEY", Description: "Reviewer key", Required: false, Secret: true},
	{Name: "HMAC_SECRET", Description: "HMAC secret", Required: true, Secret: true, CanGenerate: true},
	{Name: "DEBUG", Description: "Debug", Required: false, Default: "false", Validation: core.ValidationBoolean},
}

func newTestIO() (*bufio.Reader, *os.File) {
	return bufio.NewReader(strings.NewReader("")), nil
}

func makeOutput(t *testing.T) *os.File {
	f, err := os.CreateTemp("", "test*.out")
	require.NoError(t, err)
	t.Cleanup(func() { os.Remove(f.Name()); f.Close() })
	return f
}

func TestCLI_SetValue(t *testing.T) {
	w := core.NewWizard(cliTestVars)
	input := strings.NewReader("3000\nsecret\ns\ns\nsave\ny\n")
	output := makeOutput(t)

	c := envcli.NewWithIO(w, bufio.NewReader(input), output)
	err := c.Run()
	assert.NoError(t, err)
	assert.Equal(t, "3000", w.GetValue("PORT"))
	assert.Equal(t, "secret", w.GetValue("ADMIN_KEY"))
}

func TestCLI_SkipOptional(t *testing.T) {
	w := core.NewWizard(cliTestVars)
	input := strings.NewReader("8080\nsecret\ns\ns\nsave\ny\n")
	output := makeOutput(t)

	c := envcli.NewWithIO(w, bufio.NewReader(input), output)
	err := c.Run()
	assert.NoError(t, err)
	assert.Equal(t, "8080", w.GetValue("PORT"))
	assert.True(t, w.IsSkipped("REVIEWER_KEY"))
}

func TestCLI_Quit(t *testing.T) {
	w := core.NewWizard(cliTestVars)
	input := strings.NewReader("q\n")
	output := makeOutput(t)

	c := envcli.NewWithIO(w, bufio.NewReader(input), output)
	err := c.Run()
	assert.NoError(t, err)
}

func TestCLI_ValidationRejectsInvalid(t *testing.T) {
	w := core.NewWizard(cliTestVars)
	input := strings.NewReader("abc\n8080\nsecret\ns\ns\nsave\ny\n")
	output := makeOutput(t)

	c := envcli.NewWithIO(w, bufio.NewReader(input), output)
	err := c.Run()
	assert.NoError(t, err)
	assert.Equal(t, "8080", w.GetValue("PORT"))
}

func TestCLI_Navigation(t *testing.T) {
	w := core.NewWizard(cliTestVars)
	input := strings.NewReader("n\nn\np\nq\n")
	output := makeOutput(t)

	c := envcli.NewWithIO(w, bufio.NewReader(input), output)
	err := c.Run()
	assert.NoError(t, err)
}

func TestCLI_SaveNoThenQuit(t *testing.T) {
	w := core.NewWizard(cliTestVars)
	input := strings.NewReader("save\nn\nq\n")
	output := makeOutput(t)

	c := envcli.NewWithIO(w, bufio.NewReader(input), output)
	err := c.Run()
	assert.NoError(t, err)
}

func TestCLI_EmptyInputUsesDefault(t *testing.T) {
	w := core.NewWizard(cliTestVars)
	w.SetValue("PORT", "8080")
	w.SetValue("ADMIN_KEY", "key")
	w.SetValue("HMAC_SECRET", "secret")
	w.GoToStep(4)
	input := strings.NewReader("\nq\n")
	output := makeOutput(t)

	c := envcli.NewWithIO(w, bufio.NewReader(input), output)
	err := c.Run()
	assert.NoError(t, err)
}

func TestCLI_SaveWithoutRequired(t *testing.T) {
	w := core.NewWizard(cliTestVars)
	input := strings.NewReader("save\ny\n")
	output := makeOutput(t)

	c := envcli.NewWithIO(w, bufio.NewReader(input), output)
	err := c.Run()
	assert.NoError(t, err)
}

func TestNew(t *testing.T) {
	w := core.NewWizard(cliTestVars)
	c := envcli.New(w)
	assert.NotNil(t, c)
}
