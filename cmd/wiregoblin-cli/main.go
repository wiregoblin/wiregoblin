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
	var verbosity int
	var jsonOutput bool
	fs.StringVarP(&projectPath, "project", "p", "", "Path to the project YAML config file")
	fs.CountVarP(&verbosity, "verbose", "v", "Increase execution detail level: -v, -vv, -vvv")
	fs.BoolVar(&jsonOutput, "json", false, "Print workflow results as JSON to stdout")

	if err := fs.Parse(args); err != nil {
		if err != pflag.ErrHelp {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}

	if verbosity > 3 {
		verbosity = 3
	}
	if jsonOutput {
		verbosity = 0
	}

	positional := fs.Args()
	if len(positional) != 1 {
		fmt.Fprintf(os.Stderr, "run expects exactly one <workflow_id>\n\n%s", runUsageText)
		os.Exit(1)
	}

	workflowName := positional[0]

	resolved, err := resolveProjectPath(projectPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := run(resolved, workflowName, verbosity, jsonOutput, os.Stdout, os.Stderr); err != nil {
		os.Exit(1)
	}
}

func run(projectPath, workflowName string, verbosity int, jsonOutput bool, stdout, stderr io.Writer) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	app := cliapp.New(projectPath)
	return app.ExecuteWorkflow(ctx, workflowName, cliapp.ExecuteOptions{
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
  wiregoblin-cli run [flags] <workflow>

Examples:
  wiregoblin-cli run http_example
  wiregoblin-cli run -p config/myproject.yaml http_example
  wiregoblin-cli run -v -p config/myproject.yaml http_example
  wiregoblin-cli run -vv -p config/myproject.yaml http_example
  wiregoblin-cli run -vvv -p config/myproject.yaml http_example
  wiregoblin-cli run --json -p config/myproject.yaml http_example
`

const runUsageText = `Run a workflow from a project config

Usage:
  wiregoblin-cli run [flags] <workflow_id>

Flags:
  -p, --project  Path to the project YAML config file
  -v, --verbose  Increase execution detail level: -v, -vv, -vvv
      --json     Print workflow results as JSON to stdout
`
