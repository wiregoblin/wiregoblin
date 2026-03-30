package gotoblock

import (
	"context"
	"testing"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/models"
)

func TestExecuteMatchedCreatesJump(t *testing.T) {
	step := models.Step{
		Config: map[string]any{
			"variable":        "retries",
			"operator":        "<",
			"expected":        "3",
			"target_step_uid": "next-step",
			"wait_seconds":    5,
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
	if result.Jump.TargetStepUID != "next-step" {
		t.Fatalf("TargetStepUID = %q, want %q", result.Jump.TargetStepUID, "next-step")
	}
	if result.Jump.WaitSeconds != 5 {
		t.Fatalf("WaitSeconds = %d, want 5", result.Jump.WaitSeconds)
	}
}

func TestExecuteUnmatchedSkipsJump(t *testing.T) {
	step := models.Step{
		Config: map[string]any{
			"variable":        "retries",
			"operator":        "<",
			"expected":        "3",
			"target_step_uid": "next-step",
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
