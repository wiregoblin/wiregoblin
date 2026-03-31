package setvars

import (
	"context"
	"reflect"
	"testing"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/model"
)

func TestExecuteKeepsResolvedAssignments(t *testing.T) {
	step := model.Step{
		Config: map[string]any{
			"set": map[string]any{
				"$FromVar":   "value-1",
				"$FromConst": "https://api.example.com",
				"$Literal":   "raw",
			},
		},
	}

	result, err := New().Execute(context.Background(), &block.RunContext{}, step)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	wantExports := map[string]string{
		"$FromVar":   "value-1",
		"$FromConst": "https://api.example.com",
		"$Literal":   "raw",
	}
	if !reflect.DeepEqual(result.Exports, wantExports) {
		t.Fatalf("Exports = %#v, want %#v", result.Exports, wantExports)
	}
}

func TestExecuteWithoutRunContextKeepsOriginalValue(t *testing.T) {
	step := model.Step{
		Config: map[string]any{
			"set": map[string]any{
				"$Target": "@Source",
			},
		},
	}

	result, err := New().Execute(context.Background(), nil, step)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Exports["$Target"] != "@Source" {
		t.Fatalf("Exports[$Target] = %q, want %q", result.Exports["$Target"], "@Source")
	}
}
