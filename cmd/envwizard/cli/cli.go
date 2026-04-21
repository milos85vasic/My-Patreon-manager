package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/milos85vasic/My-Patreon-Manager/internal/envwizard/core"
	"github.com/milos85vasic/My-Patreon-Manager/internal/envwizard/definitions"
)

type CLI struct {
	wizard *core.Wizard
	reader *bufio.Reader
	output *os.File
}

func New(w *core.Wizard) *CLI {
	return &CLI{
		wizard: w,
		reader: bufio.NewReader(os.Stdin),
		output: os.Stdout,
	}
}

func NewWithIO(w *core.Wizard, input *bufio.Reader, output *os.File) *CLI {
	return &CLI{
		wizard: w,
		reader: input,
		output: output,
	}
}

func (c *CLI) Run() error {
	for {
		v := c.wizard.CurrentVar()
		if v == nil {
			break
		}

		c.displayVar(v)
		c.displayProgress()

		input, err := c.reader.ReadString('\n')
		if err != nil {
			return err
		}
		input = strings.TrimSpace(input)

		switch strings.ToLower(input) {
		case "q", "quit", "exit":
			return nil
		case "n", "next":
			c.wizard.Next()
		case "p", "prev", "previous":
			c.wizard.Previous()
		case "s", "skip":
			c.wizard.Skip(v.Name)
			c.wizard.Next()
		case "save":
			return c.save()
		case "":
			if c.wizard.IsSet(v.Name) {
				c.wizard.Next()
			} else if v.Default != "" {
				c.wizard.SetValue(v.Name, v.Default)
				c.wizard.Next()
			}
		default:
			if err := definitions.ValidateValue(v, input); err != nil {
				fmt.Fprintf(c.output, "  Invalid: %s\n", err)
				continue
			}
			c.wizard.SetValue(v.Name, input)
			c.wizard.Next()
		}
	}

	return c.save()
}

func (c *CLI) displayVar(v *core.EnvVar) {
	fmt.Fprintf(c.output, "\n--- %s ---\n", v.Name)
	fmt.Fprintf(c.output, "  %s\n", v.Description)
	if v.Required {
		fmt.Fprintf(c.output, "  [REQUIRED]")
	}
	if v.Secret {
		fmt.Fprintf(c.output, " [SECRET]")
	}
	if v.Default != "" {
		fmt.Fprintf(c.output, " (default: %s)", v.Default)
	}
	if v.CanGenerate {
		fmt.Fprintf(c.output, " [can auto-generate]")
	}
	fmt.Fprintln(c.output)
	if v.URL != "" {
		fmt.Fprintf(c.output, "  Get value: %s\n", v.URL)
	}
	if c.wizard.IsSet(v.Name) {
		if v.Secret {
			fmt.Fprintf(c.output, "  Current: ********\n")
		} else {
			fmt.Fprintf(c.output, "  Current: %s\n", c.wizard.GetValue(v.Name))
		}
	}
	fmt.Fprintf(c.output, "  Enter value (or n=next, p=prev, s=skip, save, q=quit): ")
}

func (c *CLI) displayProgress() {
	completed, total := c.wizard.Progress()
	fmt.Fprintf(c.output, "  [%d/%d completed]\n", completed, total)
}

func (c *CLI) save() error {
	missing := c.wizard.MissingRequired()
	if len(missing) > 0 {
		fmt.Fprintf(c.output, "\nWarning: %d required variables still missing:\n", len(missing))
		for _, v := range missing {
			fmt.Fprintf(c.output, "  - %s: %s\n", v.Name, v.Description)
		}
		fmt.Fprintf(c.output, "Save anyway? (y/N): ")
		input, _ := c.reader.ReadString('\n')
		if strings.ToLower(strings.TrimSpace(input)) != "y" {
			return nil
		}
	}

	gen := core.NewGenerator(c.wizard)
	output := gen.ProduceEnvFile(true)
	fmt.Fprintf(c.output, "\n--- Generated .env ---\n%s\n--- End ---\n", output)
	return nil
}
