package container

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wiregoblin/wiregoblin/internal/model"
)

func TestExecuteSuccess(t *testing.T) {
	dockerPath := writeFakeDocker(t)

	result, err := New().Execute(context.Background(), nil, model.Step{
		Config: map[string]any{
			"docker_path":     dockerPath,
			"image":           "alpine:3.20",
			"command":         "echo hi",
			"timeout_seconds": 3,
			"env":             map[string]any{"TOKEN": "secret"},
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Exports["exitCode"] != "0" {
		t.Fatalf("exitCode = %q, want 0", result.Exports["exitCode"])
	}
	if !strings.Contains(result.Exports["stdout"], "-e TOKEN=secret") {
		t.Fatalf("stdout = %q, want env arg", result.Exports["stdout"])
	}
}

func TestExecuteReturnsCommandFailure(t *testing.T) {
	dockerPath := writeFakeDocker(t)

	result, err := New().Execute(context.Background(), nil, model.Step{
		Config: map[string]any{
			"docker_path":     dockerPath,
			"image":           "alpine:3.20",
			"command":         "__FAIL__",
			"timeout_seconds": 3,
		},
	})
	if err == nil {
		t.Fatal("Execute() error = nil, want failure")
	}
	if !strings.Contains(err.Error(), "exit code 7") {
		t.Fatalf("error = %q", err.Error())
	}
	if result.Exports["exitCode"] != "7" {
		t.Fatalf("exitCode = %q, want 7", result.Exports["exitCode"])
	}
}

func writeFakeDocker(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "docker")
	script := "#!/bin/sh\n" +
		"for arg in \"$@\"; do\n" +
		"  if [ \"$arg\" = \"__FAIL__\" ]; then\n" +
		"    echo boom 1>&2\n" +
		"    exit 7\n" +
		"  fi\n" +
		"done\n" +
		"printf '%s' \"$*\"\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}
