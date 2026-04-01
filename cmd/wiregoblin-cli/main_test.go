package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wiregoblin/wiregoblin/internal/model"
)

func ptrStr(s string) *string { return &s }

func TestResolveProjectPathFromDir_ExplicitPath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path, err := resolveProjectPathFromDir("config/custom.yaml", dir)
	if err != nil {
		t.Fatalf("resolveProjectPathFromDir returned error: %v", err)
	}
	expected, err := filepath.Abs(filepath.Join(dir, "config/custom.yaml"))
	if err != nil {
		t.Fatalf("filepath.Abs returned error: %v", err)
	}
	if path != expected {
		t.Fatalf("resolveProjectPathFromDir returned %q, want %q", path, expected)
	}
}

func TestResolveProjectPathFromDir_FindsYAML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	expected := filepath.Join(dir, "wiregoblin.yaml")
	if err := os.WriteFile(expected, []byte("name: test\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	path, err := resolveProjectPathFromDir("", dir)
	if err != nil {
		t.Fatalf("resolveProjectPathFromDir returned error: %v", err)
	}
	if path != expected {
		t.Fatalf("resolveProjectPathFromDir returned %q, want %q", path, expected)
	}
}

func TestResolveProjectPathFromDir_FindsYML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	expected := filepath.Join(dir, "wiregoblin.yml")
	if err := os.WriteFile(expected, []byte("name: test\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	path, err := resolveProjectPathFromDir("", dir)
	if err != nil {
		t.Fatalf("resolveProjectPathFromDir returned error: %v", err)
	}
	if path != expected {
		t.Fatalf("resolveProjectPathFromDir returned %q, want %q", path, expected)
	}
}

func TestResolveProjectPathFromDir_NotFound(t *testing.T) {
	t.Parallel()

	_, err := resolveProjectPathFromDir("", t.TempDir())
	if err == nil {
		t.Fatal("resolveProjectPathFromDir returned nil error, want error")
	}
}

func TestRunStreamsWorkflowEventsAsJSON(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "wiregoblin.yaml")
	config := `id: demo
name: Demo
workflows:
  alpha:
    name: Alpha
    blocks:
      - id: wait
        name: Wait
        type: delay
        milliseconds: 1
`
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	execErr := run(configPath, ptrStr("alpha"), 0, true, &stdout, &stderr)
	if execErr != nil {
		t.Fatalf("run returned error: %v", execErr)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	events := decodeEvents(t, stdout.String())
	if len(events) != 4 {
		t.Fatalf("len(events) = %d, want 4", len(events))
	}
	if events[0].Type != model.EventWorkflowStarted {
		t.Fatalf("events[0].Type = %q, want %q", events[0].Type, model.EventWorkflowStarted)
	}
	if events[1].Type != model.EventStepStarted {
		t.Fatalf("events[1].Type = %q, want %q", events[1].Type, model.EventStepStarted)
	}
	if events[2].Type != model.EventStepFinished {
		t.Fatalf("events[2].Type = %q, want %q", events[2].Type, model.EventStepFinished)
	}
	if events[3].Type != model.EventWorkflowFinished {
		t.Fatalf("events[3].Type = %q, want %q", events[3].Type, model.EventWorkflowFinished)
	}
	if events[3].Status != "ok" {
		t.Fatalf("events[3].Status = %q, want ok", events[3].Status)
	}
}

func TestRunStreamsProgressToStderr(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "wiregoblin.yaml")
	config := `id: demo
name: Demo
workflows:
  alpha:
    name: Alpha
    blocks:
      - id: wait
        name: Wait
        type: delay
        milliseconds: 1
`
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	execErr := run(configPath, ptrStr("alpha"), 0, false, &stdout, &stderr)
	if execErr != nil {
		t.Fatalf("run returned error: %v", execErr)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(
		stderr.String(),
		"🧌 Goblin crew enters \"Alpha\" from project \"Demo\". 1 step packed.",
	) {
		t.Fatalf("stderr = %q, want workflow start line", stderr.String())
	}
	if !strings.Contains(stderr.String(), "[1/1] Goblin pokes \"Wait\" [delay]") {
		t.Fatalf("stderr = %q, want step progress line", stderr.String())
	}
	if !strings.Contains(stderr.String(), "🧌 Goblin crew hauled \"Alpha\" out of the cave in ") ||
		!strings.Contains(stderr.String(), "✅ 1/1 passed.") {
		t.Fatalf("stderr = %q, want completion line", stderr.String())
	}
}

func TestRunVerboseShowsGoblinDetails(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "wiregoblin.yaml")
	config := `id: demo
name: Demo
workflows:
  alpha:
    name: Alpha
    blocks:
      - id: wait
        name: Wait
        type: delay
        milliseconds: 1
`
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	execErr := run(configPath, ptrStr("alpha"), 3, false, &stdout, &stderr)
	if execErr != nil {
		t.Fatalf("run returned error: %v", execErr)
	}

	output := stderr.String()
	if !strings.Contains(output, "✅ Goblin loot secured") {
		t.Fatalf("stderr = %q, want goblin success line", output)
	}
	if !strings.Contains(output, "📜 Goblin spellbook:") {
		t.Fatalf("stderr = %q, want request section", output)
	}
	if !strings.Contains(output, "💎 Goblin stash:") {
		t.Fatalf("stderr = %q, want response section", output)
	}
}

func TestRunIndentsNestedWorkflowSteps(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "wiregoblin.yaml")
	config := `id: demo
name: Demo
workflows:
  child:
    name: Child
    blocks:
      - id: wait
        name: Child Wait
        type: delay
        milliseconds: 1
  parent:
    name: Parent
    blocks:
      - id: nested
        name: Nested Workflow
        type: workflow
        target_workflow_id: child
`
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	execErr := run(configPath, ptrStr("parent"), 0, false, &stdout, &stderr)
	if execErr != nil {
		t.Fatalf("run returned error: %v", execErr)
	}

	output := stderr.String()
	if !strings.Contains(output, "[1/1] Goblin pokes \"Nested Workflow\" [workflow]") {
		t.Fatalf("stderr = %q, want parent workflow step", output)
	}
	if !strings.Contains(output, "  [1/1] Goblin pokes \"Child Wait\" [delay]") {
		t.Fatalf("stderr = %q, want indented nested step", output)
	}
}

func TestRunWithoutWorkflowReturnsNotFound(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "wiregoblin.yaml")
	config := `id: demo
name: Demo
workflows:
  beta:
    name: Beta
    blocks:
      - id: wait
        type: delay
        milliseconds: 1
  alpha:
    name: Alpha
    blocks:
      - id: wait
        type: delay
        milliseconds: 1
`
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	execErr := run(configPath, ptrStr(""), 0, true, &stdout, &stderr)
	if execErr == nil {
		t.Fatal("run returned nil error, want error")
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if got := execErr.Error(); got != "workflow \"\" not found; available: [alpha beta]" {
		t.Fatalf("error = %q, want %q", got, "workflow \"\" not found; available: [alpha beta]")
	}
}

func decodeEvents(t *testing.T, output string) []model.RunEvent {
	t.Helper()

	scanner := bufio.NewScanner(strings.NewReader(output))
	events := make([]model.RunEvent, 0)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event model.RunEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("Unmarshal event %q: %v", line, err)
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("Scanner error: %v", err)
	}
	return events
}
