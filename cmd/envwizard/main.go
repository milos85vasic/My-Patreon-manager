package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/milos85vasic/My-Patreon-Manager/cmd/envwizard/cli"
	envapi "github.com/milos85vasic/My-Patreon-Manager/cmd/envwizard/api"
	"github.com/milos85vasic/My-Patreon-Manager/cmd/envwizard/web"
	"github.com/milos85vasic/My-Patreon-Manager/internal/envwizard/core"
	"github.com/milos85vasic/My-Patreon-Manager/internal/envwizard/definitions"
)

func main() {
	var (
		cliMode bool
		webMode bool
		apiMode bool
		webAddr string
		apiAddr string
		envPath string
		profile string
	)

	flag.BoolVar(&cliMode, "cli", false, "Force CLI mode")
	flag.BoolVar(&webMode, "web", false, "Start web UI")
	flag.BoolVar(&apiMode, "api", false, "Start REST API only")
	flag.StringVar(&webAddr, "web-addr", ":8080", "Web UI address")
	flag.StringVar(&apiAddr, "api-addr", ":8081", "REST API address")
	flag.StringVar(&envPath, "env", "", "Load existing .env file")
	flag.StringVar(&profile, "profile", "", "Load profile")
	flag.Parse()

	vars := definitions.GetAll()
	var wizard *core.Wizard
	var err error

	if envPath != "" {
		wizard, err = core.NewWizardFromEnvFile(vars, envPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading %s: %v\n", envPath, err)
			os.Exit(1)
		}
		fmt.Printf("Loaded existing values from %s\n", envPath)
	} else if profile != "" {
		p, loadErr := core.LoadProfile(profile)
		if loadErr != nil {
			fmt.Fprintf(os.Stderr, "Error loading profile %s: %v\n", profile, loadErr)
			os.Exit(1)
		}
		wizard = core.NewWizard(vars)
		for k, v := range p.Values {
			wizard.SetValue(k, v)
		}
		fmt.Printf("Loaded profile: %s\n", profile)
	} else {
		wizard = core.NewWizard(vars)
	}

	switch {
	case webMode:
		srv := web.NewServer(wizard)
		fmt.Printf("EnvWizard web UI on %s\n", webAddr)
		if listenErr := http.ListenAndServe(webAddr, srv); listenErr != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", listenErr)
			os.Exit(1)
		}
	case apiMode:
		srv := envapi.NewServer(wizard)
		fmt.Printf("EnvWizard REST API on %s\n", apiAddr)
		if listenErr := http.ListenAndServe(apiAddr, srv); listenErr != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", listenErr)
			os.Exit(1)
		}
	default:
		c := cli.New(wizard)
		if runErr := c.Run(); runErr != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", runErr)
			os.Exit(1)
		}
	}
}
