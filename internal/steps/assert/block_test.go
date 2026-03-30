package assert

import (
	"context"
	"testing"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/models"
)

func TestExecuteSuccess(t *testing.T) {
	step := models.Step{
		Config: map[string]any{
			"variable": "status",
			"operator": "=",
			"expected": "ok",
		},
	}

	result, err := New().Execute(context.Background(), &block.RunContext{
		Variables: map[string]string{"status": "ok"},
	}, step)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output, ok := result.Output.(map[string]any)
	if !ok {
		t.Fatalf("Output type = %T, want map[string]any", result.Output)
	}
	if output["passed"] != true {
		t.Fatalf("passed = %v, want true", output["passed"])
	}
	if output["actual"] != "ok" {
		t.Fatalf("actual = %v, want ok", output["actual"])
	}
}

func TestExecuteFailureReturnsOutput(t *testing.T) {
	step := models.Step{
		Config: map[string]any{
			"variable":      "status",
			"operator":      "=",
			"expected":      "ok",
			"error_message": " custom error ",
		},
	}

	result, err := New().Execute(context.Background(), &block.RunContext{
		Variables: map[string]string{"status": "failed"},
	}, step)
	if err == nil {
		t.Fatal("Execute() error = nil, want failure")
	}
	if err.Error() != "custom error" {
		t.Fatalf("error = %q, want %q", err.Error(), "custom error")
	}

	output := result.Output.(map[string]any)
	if output["passed"] != false {
		t.Fatalf("passed = %v, want false", output["passed"])
	}
}
