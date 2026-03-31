// Package container implements Docker-backed workflow steps.
package container

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/model"
)

const blockType = "container"

// Block executes one docker container job.
type Block struct{}

// New creates a container workflow block.
func New() *Block {
	return &Block{}
}

// Type returns the block type identifier.
func (b *Block) Type() model.BlockType {
	return blockType
}

// SupportsResponseMapping reports whether the block exposes response mapping.
func (b *Block) SupportsResponseMapping() bool {
	return true
}

// ReferencePolicy describes which fields accept @ references and runtime variables.
func (b *Block) ReferencePolicy() []block.ReferencePolicy {
	return []block.ReferencePolicy{
		{Field: "image", Constants: true},
		{Field: "command", Constants: true, Variables: true, InlineOnly: true},
		{Field: "env", Constants: true, Variables: true, InlineOnly: true},
	}
}

// Validate checks the minimal container fields.
func (b *Block) Validate(step model.Step) error {
	config := decodeConfig(step)
	if strings.TrimSpace(config.Image) == "" {
		return fmt.Errorf("container image is required")
	}
	if strings.TrimSpace(config.Command) == "" {
		return fmt.Errorf("container command is required")
	}
	if config.TimeoutSeconds <= 0 {
		return fmt.Errorf("container timeout seconds must be greater than zero")
	}
	if config.MountSource != "" && strings.TrimSpace(config.Workdir) == "" {
		return fmt.Errorf("container workdir is required when a mount source is set")
	}
	return nil
}

// Execute runs one docker container and returns stdout/stderr/exit code.
func (b *Block) Execute(ctx context.Context, _ *block.RunContext, step model.Step) (*block.Result, error) {
	return execute(ctx, decodeConfig(step))
}

func resolveDockerBinary(configured string) string {
	if strings.TrimSpace(configured) != "" {
		return filepath.Clean(strings.TrimSpace(configured))
	}
	if path, err := exec.LookPath("docker"); err == nil {
		return path
	}
	return "docker"
}

func execute(ctx context.Context, config containerConfig) (*block.Result, error) {
	timeout := time.Duration(config.TimeoutSeconds) * time.Second
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := []string{"run", "--rm"}
	if config.MountSource != "" {
		args = append(args, "-v", fmt.Sprintf("%s:%s", config.MountSource, config.Workdir))
		args = append(args, "-w", config.Workdir)
	}
	for key, value := range config.Env {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		args = append(args, "-e", fmt.Sprintf("%s=%s", trimmedKey, value))
	}
	args = append(args, config.Image, "sh", "-c", config.Command)

	dockerBinary := resolveDockerBinary(config.DockerPath)
	if _, err := os.Stat(dockerBinary); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("stat docker binary %q: %w", dockerBinary, err)
	}
	// #nosec G204 -- Docker binary and args are validated workflow inputs by design.
	command := exec.CommandContext(runCtx, dockerBinary, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	startedAt := time.Now()
	err := command.Run()
	durationMs := time.Since(startedAt).Milliseconds()

	output := map[string]any{
		"image":      config.Image,
		"command":    config.Command,
		"stdout":     strings.TrimSpace(stdout.String()),
		"stderr":     strings.TrimSpace(stderr.String()),
		"exitCode":   0,
		"durationMs": durationMs,
		"timedOut":   false,
	}

	exports := map[string]string{
		"stdout":     strings.TrimSpace(stdout.String()),
		"stderr":     strings.TrimSpace(stderr.String()),
		"exitCode":   "0",
		"durationMs": strconv.FormatInt(durationMs, 10),
		"timedOut":   "false",
	}

	if err == nil {
		return &block.Result{Output: output, Exports: exports}, nil
	}

	if runCtx.Err() == context.DeadlineExceeded {
		output["timedOut"] = true
		exports["timedOut"] = "true"
		return &block.Result{Output: output, Exports: exports}, fmt.Errorf(
			"container timed out after %ds",
			config.TimeoutSeconds,
		)
	}

	exitCode := extractExitCode(err)
	output["exitCode"] = exitCode
	exports["exitCode"] = strconv.Itoa(exitCode)
	if output["stderr"] == "" {
		output["stderr"] = strings.TrimSpace(err.Error())
		exports["stderr"] = strings.TrimSpace(err.Error())
	}

	stderrMsg := strings.TrimSpace(stderr.String())
	if stderrMsg == "" {
		stderrMsg = strings.TrimSpace(err.Error())
	}
	return &block.Result{Output: output, Exports: exports}, fmt.Errorf(
		"container command failed with exit code %d: %s",
		exitCode,
		stderrMsg,
	)
}

func extractExitCode(err error) int {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return 1
	}
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok {
		return 1
	}
	return status.ExitStatus()
}
