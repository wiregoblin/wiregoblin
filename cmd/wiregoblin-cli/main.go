// Package main exposes the WireGoblin CLI entrypoint.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/spf13/pflag"

	cliapp "github.com/wiregoblin/wiregoblin/internal/app/cli"
	workflowservice "github.com/wiregoblin/wiregoblin/internal/service/workflow"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usageText)
		os.Exit(1)
	}

	switch os.Args[1] {
	case "run":
		runCmd(os.Args[2:])
	case "-h", "--help", "help":
		fmt.Fprint(os.Stderr, usageText)
	default:
		_, _ = io.WriteString(os.Stderr, "unknown command ")
		// #nosec G705 -- command name is rendered to stderr as quoted plain text.
		_, _ = io.WriteString(os.Stderr, strconv.Quote(os.Args[1]))
		_, _ = io.WriteString(os.Stderr, "\n\n")
		_, _ = io.WriteString(os.Stderr, usageText)
		os.Exit(1)
	}
}

func runCmd(args []string) {
	fs := pflag.NewFlagSet("run", pflag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(os.Stderr, runUsageText) }

	var projectPath string
	var envFile string
	var verbosity int
	var jsonOutput bool
	fs.StringVarP(&projectPath, "project", "p", "", "Path to the project YAML config file")
	fs.StringVarP(&envFile, "env", "e", "", "Path to a .env file to load before running")
	fs.CountVarP(&verbosity, "verbose", "v", "Increase execution detail level: -v, -vv, -vvv")
	fs.BoolVar(&jsonOutput, "json", false, "Print workflow results as JSON to stdout")

	if err := fs.Parse(args); err != nil {
		if err != pflag.ErrHelp {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}

	if envFile != "" {
		if err := godotenv.Load(envFile); err != nil {
			fmt.Fprintf(os.Stderr, "load env file %s: %v\n", envFile, err)
			os.Exit(1)
		}
	}

	if verbosity > 3 {
		verbosity = 3
	}
	if jsonOutput {
		verbosity = 0
	}

	positional := fs.Args()
	if len(positional) > 1 {
		fmt.Fprintf(os.Stderr, "run expects at most one <workflow_id>\n\n%s", runUsageText)
		os.Exit(1)
	}

	resolved, err := resolveProjectPath(projectPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	var workflowID *string
	if len(positional) == 1 {
		workflowID = &positional[0]
	}

	if err := run(resolved, workflowID, verbosity, jsonOutput, os.Stdout, os.Stderr); err != nil {
		os.Exit(1)
	}
}

func run(projectPath string, workflowID *string, verbosity int, jsonOutput bool, stdout, stderr io.Writer) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	app := cliapp.New(projectPath)
	return app.Run(ctx, workflowID, cliapp.ExecuteOptions{
		RunOptions: workflowservice.RunOptions{Context: ctx},
		Verbosity:  verbosity,
		JSONOutput: jsonOutput,
		Stdout:     stdout,
		Stderr:     stderr,
	})
}

func resolveProjectPath(projectPath string) (string, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("detect current directory: %w", err)
	}
	return resolveProjectPathFromDir(projectPath, workingDir)
}

func resolveProjectPathFromDir(projectPath, workingDir string) (string, error) {
	if strings.TrimSpace(projectPath) != "" {
		if filepath.IsAbs(projectPath) {
			return projectPath, nil
		}
		return filepath.Abs(filepath.Join(workingDir, projectPath))
	}

	for _, candidate := range []string{"wiregoblin.yaml", "wiregoblin.yml"} {
		path := filepath.Join(workingDir, candidate)
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() {
			return path, nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("check project config %s: %w", path, err)
		}
	}

	return "", fmt.Errorf("project config not found in %s; use -p to specify a file", workingDir)
}

const usageText = `WireGoblin workflow runner

Usage:
  wiregoblin-cli run [flags] [workflow]

Examples:
  wiregoblin-cli run                              # run all workflows in project order
  wiregoblin-cli run http_example
  wiregoblin-cli run -p config/myproject.yaml     # run all workflows from a specific config
  wiregoblin-cli run -p config/myproject.yaml http_example
  wiregoblin-cli run -e .env -p config/myproject.yaml http_example
  wiregoblin-cli run -v -p config/myproject.yaml http_example
  wiregoblin-cli run --json -p config/myproject.yaml http_example
`

const runUsageText = `Run a workflow (or all workflows) from a project config

Usage:
  wiregoblin-cli run [flags] [workflow_id]

If workflow_id is omitted, all workflows are run sequentially in alphabetical order.

Flags:
  -p, --project  Path to the project YAML config file
  -e, --env      Path to a .env file to load before running
  -v, --verbose  Increase execution detail level: -v, -vv, -vvv
      --json     Print workflow results as JSON to stdout
`
