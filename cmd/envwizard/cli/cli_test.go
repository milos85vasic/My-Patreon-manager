package cli_test

import (
	"bufio"
	"os"
	"strings"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/envwizard/core"
	envcli "github.com/milos85vasic/My-Patreon-Manager/cmd/envwizard/cli"
	"github.com/stretchr/testify/assert"
)

var cliTestVars = []*core.EnvVar{
	{Name: "PORT", Description: "HTTP port", Required: true, Validation: core.ValidationPort, Default: "8080"},
	{Name: "ADMIN_KEY", Description: "Admin key", Required: true, Secret: true},
	{Name: "DEBUG", Description: "Debug", Required: false, Default: "false", Validation: core.ValidationBoolean},
}

func TestCLI_SetValue(t *testing.T) {
	w := core.NewWizard(cliTestVars)
	input := strings.NewReader("3000\nsecret\ntrue\nsave\ny\n")
	output, _ := os.CreateTemp("", "test*.out")
	defer os.Remove(output.Name())

	c := envcli.NewWithIO(w, bufio.NewReader(input), output)
	err := c.Run()
	assert.NoError(t, err)
	assert.Equal(t, "3000", w.GetValue("PORT"))
	assert.Equal(t, "secret", w.GetValue("ADMIN_KEY"))
}

func TestCLI_SkipOptional(t *testing.T) {
	w := core.NewWizard(cliTestVars)
	input := strings.NewReader("8080\nsecret\ns\nsave\ny\n")
	output, _ := os.CreateTemp("", "test*.out")
	defer os.Remove(output.Name())

	c := envcli.NewWithIO(w, bufio.NewReader(input), output)
	err := c.Run()
	assert.NoError(t, err)
	assert.Equal(t, "8080", w.GetValue("PORT"))
	assert.True(t, w.IsSkipped("DEBUG"))
}

func TestCLI_Quit(t *testing.T) {
	w := core.NewWizard(cliTestVars)
	input := strings.NewReader("q\n")
	output, _ := os.CreateTemp("", "test*.out")
	defer os.Remove(output.Name())

	c := envcli.NewWithIO(w, bufio.NewReader(input), output)
	err := c.Run()
	assert.NoError(t, err)
}

func TestCLI_ValidationRejectsInvalid(t *testing.T) {
	w := core.NewWizard(cliTestVars)
	input := strings.NewReader("abc\n8080\nsecret\ntrue\nsave\ny\n")
	output, _ := os.CreateTemp("", "test*.out")
	defer os.Remove(output.Name())

	c := envcli.NewWithIO(w, bufio.NewReader(input), output)
	err := c.Run()
	assert.NoError(t, err)
	assert.Equal(t, "8080", w.GetValue("PORT"))
}
