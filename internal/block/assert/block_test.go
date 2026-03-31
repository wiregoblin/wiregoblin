package assert

import (
	"context"
	"testing"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/model"
)

func TestExecuteSuccess(t *testing.T) {
	step := model.Step{
		Config: map[string]any{
			"variable": "$status",
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
	step := model.Step{
		Config: map[string]any{
			"variable":      "$status",
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

func TestExecuteReadsSecretVariablesByExplicitReference(t *testing.T) {
	step := model.Step{
		Config: map[string]any{
			"variable": "$session_token",
			"operator": "=",
			"expected": "secret-demo-token",
		},
	}

	result, err := New().Execute(context.Background(), &block.RunContext{
		SecretVariables: map[string]string{"session_token": "secret-demo-token"},
	}, step)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := result.Output.(map[string]any)
	if output["actual"] != "secret-demo-token" {
		t.Fatalf("actual = %v, want secret-demo-token", output["actual"])
	}
}

func TestExecuteReadsVariableFromInterpolatedName(t *testing.T) {
	step := model.Step{
		Config: map[string]any{
			"variable": "$cached_!Each.Item",
			"operator": "=",
			"expected": "Alice",
		},
	}

	result, err := New().Execute(context.Background(), &block.RunContext{
		Variables: map[string]string{"cached_101": "Alice"},
		Builtins:  map[string]string{"Each.Item": "101"},
	}, step)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := result.Output.(map[string]any)
	if output["variable"] != "$cached_101" {
		t.Fatalf("variable = %v, want $cached_101", output["variable"])
	}
	if output["actual"] != "Alice" {
		t.Fatalf("actual = %v, want Alice", output["actual"])
	}
}
