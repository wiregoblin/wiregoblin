package logblock

import (
	"context"
	"testing"

	"github.com/wiregoblin/wiregoblin/internal/model"
)

func TestExecuteReturnsResolvedPayload(t *testing.T) {
	result, err := New().Execute(context.Background(), nil, model.Step{
		Config: map[string]any{
			"message": "hello",
			"level":   "warn",
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := result.Output.(map[string]any)
	if output["message"] != "hello" {
		t.Fatalf("message = %v, want hello", output["message"])
	}
	if output["level"] != "warn" {
		t.Fatalf("level = %v, want warn", output["level"])
	}
	if len(result.Exports) != 0 {
		t.Fatalf("Exports = %#v, want none", result.Exports)
	}
}

func TestValidateDefaultsLevelAndRejectsInvalidValues(t *testing.T) {
	blk := New()
	if err := blk.Validate(model.Step{Config: map[string]any{"message": "hello"}}); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if err := blk.Validate(model.Step{Config: map[string]any{"message": "hello", "level": "trace"}}); err == nil {
		t.Fatal("Validate() error = nil, want invalid level error")
	}
}
