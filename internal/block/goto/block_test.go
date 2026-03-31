package gotoblock

import (
	"context"
	"testing"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/model"
)

func TestExecuteMatchedCreatesJump(t *testing.T) {
	step := model.Step{
		Config: map[string]any{
			"variable":       "$retries",
			"operator":       "<",
			"expected":       "3",
			"target_step_id": "next-step",
			"wait_seconds":   5,
		},
	}

	result, err := New().Execute(context.Background(), &block.RunContext{
		Variables: map[string]string{"retries": "2"},
	}, step)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Jump == nil {
		t.Fatal("Jump = nil, want jump")
	}
	if result.Jump.TargetStepID != "next-step" {
		t.Fatalf("TargetStepID = %q, want %q", result.Jump.TargetStepID, "next-step")
	}
	if result.Jump.WaitSeconds != 5 {
		t.Fatalf("WaitSeconds = %d, want 5", result.Jump.WaitSeconds)
	}
}

func TestExecuteUnmatchedSkipsJump(t *testing.T) {
	step := model.Step{
		Config: map[string]any{
			"variable":       "$retries",
			"operator":       "<",
			"expected":       "3",
			"target_step_id": "next-step",
		},
	}

	result, err := New().Execute(context.Background(), &block.RunContext{
		Variables: map[string]string{"retries": "4"},
	}, step)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Jump != nil {
		t.Fatalf("Jump = %#v, want nil", result.Jump)
	}

	output := result.Output.(map[string]any)
	if output["matched"] != false {
		t.Fatalf("matched = %v, want false", output["matched"])
	}
}

func TestExecuteReadsVariableNameFromBuiltinInterpolation(t *testing.T) {
	step := model.Step{
		Config: map[string]any{
			"variable":       "$status_!ErrorBlockID",
			"operator":       "=",
			"expected":       "retry",
			"target_step_id": "handle_db_error",
		},
	}

	result, err := New().Execute(context.Background(), &block.RunContext{
		Variables: map[string]string{"status_seed_postgres": "retry"},
		Builtins:  map[string]string{"ErrorBlockID": "seed_postgres"},
	}, step)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Jump == nil {
		t.Fatal("Jump = nil, want jump")
	}
	if result.Jump.TargetStepID != "handle_db_error" {
		t.Fatalf("TargetStepID = %q, want %q", result.Jump.TargetStepID, "handle_db_error")
	}

	output := result.Output.(map[string]any)
	if output["variable"] != "$status_seed_postgres" {
		t.Fatalf("variable = %v, want $status_seed_postgres", output["variable"])
	}
}

func TestExecuteReadsInterpolatedVariableName(t *testing.T) {
	step := model.Step{
		Config: map[string]any{
			"variable":       "$status_!Retry.Attempt",
			"operator":       "=",
			"expected":       "ready",
			"target_step_id": "next-step",
		},
	}

	result, err := New().Execute(context.Background(), &block.RunContext{
		Variables: map[string]string{"status_2": "ready"},
		Builtins:  map[string]string{"Retry.Attempt": "2"},
	}, step)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Jump == nil {
		t.Fatal("Jump = nil, want jump")
	}

	output := result.Output.(map[string]any)
	if output["variable"] != "$status_2" {
		t.Fatalf("variable = %v, want $status_2", output["variable"])
	}
	if output["actual"] != "ready" {
		t.Fatalf("actual = %v, want ready", output["actual"])
	}
}
