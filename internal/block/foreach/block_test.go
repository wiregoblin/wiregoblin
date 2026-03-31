package foreach

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/model"
)

func TestExecuteIteratesJSONItemsAndExposesBuiltins(t *testing.T) {
	seen := []string{}
	var runCtx *block.RunContext
	runCtx = &block.RunContext{
		Builtins: map[string]string{"Keep": "value"},
		ExecuteStep: func(_ context.Context, _ model.Step) (*block.Result, error) {
			seen = append(seen, runCtx.Builtins["Each.Item.id"])
			return &block.Result{
				Output: map[string]any{
					"id": runCtx.Builtins["Each.Item.id"],
				},
			}, nil
		},
	}

	step := model.Step{
		Name: "loop",
		Config: map[string]any{
			"items": `[{"id":"42"},{"id":"99"}]`,
			"block": map[string]any{
				"type": "http",
			},
		},
	}

	result, err := New().Execute(context.Background(), runCtx, step)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := result.Output.(map[string]any)
	results := output["results"].([]map[string]any)
	if len(seen) != 2 || seen[0] != "42" || seen[1] != "99" {
		t.Fatalf("seen = %#v, want [42 99]", seen)
	}
	first := results[0]["output"].(map[string]any)
	second := results[1]["output"].(map[string]any)
	if first["id"] != "42" {
		t.Fatalf("first id = %v", first["id"])
	}
	if second["id"] != "99" {
		t.Fatalf("second id = %v", second["id"])
	}
	if got := runCtx.Builtins["Keep"]; got != "value" {
		t.Fatalf("Keep = %q, want value", got)
	}
}

func TestExecuteCollectsValuesAndExportsJSONArrays(t *testing.T) {
	var runCtx *block.RunContext
	runCtx = &block.RunContext{
		Builtins: map[string]string{},
		ExecuteStep: func(_ context.Context, _ model.Step) (*block.Result, error) {
			return &block.Result{
				Output: map[string]any{
					"body": map[string]any{
						"id":   runCtx.Builtins["Each.Item.id"],
						"name": runCtx.Builtins["Each.Item.name"],
					},
				},
			}, nil
		},
	}

	result, err := New().Execute(context.Background(), runCtx, model.Step{
		Name: "loop",
		Config: map[string]any{
			"items": `[{"id":"42","name":"Alice"},{"id":"99","name":"Bob"}]`,
			"block": map[string]any{
				"type": "fake",
			},
			"collect": map[string]any{
				"$user_ids":   "output.body.id",
				"$user_names": "output.body.name",
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := result.Output.(map[string]any)
	collected := output["collected"].(map[string]any)
	userIDs := collected["user_ids"].([]any)
	userNames := collected["user_names"].([]any)
	if len(userIDs) != 2 || userIDs[0] != "42" || userIDs[1] != "99" {
		t.Fatalf("user_ids = %#v", userIDs)
	}
	if len(userNames) != 2 || userNames[0] != "Alice" || userNames[1] != "Bob" {
		t.Fatalf("user_names = %#v", userNames)
	}
	if got := result.Exports["$user_ids"]; got != `["42","99"]` {
		t.Fatalf("exports[$user_ids] = %q", got)
	}
}

func TestExecuteIteratesNumericRangeItems(t *testing.T) {
	seen := []string{}
	var runCtx *block.RunContext
	runCtx = &block.RunContext{
		Builtins: map[string]string{},
		ExecuteStep: func(_ context.Context, _ model.Step) (*block.Result, error) {
			seen = append(seen, runCtx.Builtins["Each.Item"])
			return &block.Result{
				Output: map[string]any{
					"value": runCtx.Builtins["Each.Item"],
					"index": runCtx.Builtins["Each.Index"],
				},
			}, nil
		},
	}

	result, err := New().Execute(context.Background(), runCtx, model.Step{
		Name: "loop",
		Config: map[string]any{
			"items": map[string]any{
				"from": 1,
				"to":   5,
				"step": 2,
			},
			"block": map[string]any{
				"type": "fake",
			},
			"collect": map[string]any{
				"$range_values": "item",
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got := fmt.Sprint(seen); got != "[1 3 5]" {
		t.Fatalf("seen = %s, want [1 3 5]", got)
	}

	output := result.Output.(map[string]any)
	results := output["results"].([]map[string]any)
	if got := results[0]["item"]; got != 1 {
		t.Fatalf("first item = %v, want 1", got)
	}
	if got := results[2]["item"]; got != 5 {
		t.Fatalf("third item = %v, want 5", got)
	}

	collected := output["collected"].(map[string]any)
	values := collected["range_values"].([]any)
	if fmt.Sprint(values) != "[1 3 5]" {
		t.Fatalf("range_values = %#v", values)
	}
	if got := result.Exports["$range_values"]; got != `[1,3,5]` {
		t.Fatalf("exports[$range_values] = %q", got)
	}
}

func TestValidateRejectsInvalidRangeStepDirection(t *testing.T) {
	err := New().Validate(model.Step{
		Name: "loop",
		Config: map[string]any{
			"items": map[string]any{
				"from": 1,
				"to":   3,
				"step": -1,
			},
			"block": map[string]any{
				"type": "fake",
			},
		},
	})
	if err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
	if got := err.Error(); got != "foreach items range step must be positive when from < to" {
		t.Fatalf("Validate() error = %q", got)
	}
}

func TestValidateRejectsParallelNestedAssignments(t *testing.T) {
	err := New().Validate(model.Step{
		Name: "loop",
		Config: map[string]any{
			"items":       `["one"]`,
			"concurrency": 2,
			"block": map[string]any{
				"type": "fake",
				"assign": map[string]any{
					"$value": "body.id",
				},
			},
		},
	})
	if err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
	wantErr := "foreach concurrency > 1 does not support nested runtime assignments; use collect instead"
	if got := err.Error(); got != wantErr {
		t.Fatalf("Validate() error = %q", got)
	}
}

func TestExecuteConcurrentPreservesOrderAndCollectsErrors(t *testing.T) {
	variables := map[string]string{"shared": "seed"}
	runCtx := &block.RunContext{
		Variables: map[string]string{"shared": "seed"},
		ExecuteStep: func(context.Context, model.Step) (*block.Result, error) {
			return nil, errors.New("unexpected sequential execution")
		},
		ExecuteIsolatedStep: func(
			_ context.Context,
			isolated *block.RunContext,
			_ model.Step,
		) (*block.Result, error) {
			index := isolated.Builtins["Each.Index"]
			if isolated.Variables["shared"] != "seed" {
				return nil, fmt.Errorf("shared = %q", isolated.Variables["shared"])
			}
			isolated.Variables["shared"] = "changed-" + index
			if index == "0" {
				time.Sleep(30 * time.Millisecond)
			}
			if index == "1" {
				return nil, errors.New("boom-1")
			}
			return &block.Result{
				Output: map[string]any{
					"body": map[string]any{"id": isolated.Builtins["Each.Item.id"]},
				},
				Exports: map[string]string{
					"status": "ok-" + index,
				},
			}, nil
		},
	}

	result, err := New().Execute(context.Background(), runCtx, model.Step{
		Name: "loop",
		Config: map[string]any{
			"items":       `[{"id":"a"},{"id":"b"},{"id":"c"}]`,
			"concurrency": 2,
			"block": map[string]any{
				"type": "fake",
			},
			"collect": map[string]any{
				"$ids":    "output.body.id",
				"$errors": "error",
				"$status": "exports.status",
			},
		},
	})
	if err == nil {
		t.Fatal("Execute() error = nil, want aggregate error")
	}
	if got := err.Error(); got != "foreach failed: 1 of 3 iterations failed: boom-1" {
		t.Fatalf("Execute() error = %q", got)
	}
	if variables["shared"] != "seed" {
		t.Fatalf("shared variable mutated: %q", variables["shared"])
	}
	if got := runCtx.Variables["shared"]; got != "seed" {
		t.Fatalf("runCtx shared = %q, want seed", got)
	}

	output := result.Output.(map[string]any)
	results := output["results"].([]map[string]any)
	if len(results) != 3 {
		t.Fatalf("results len = %d", len(results))
	}
	if results[0]["index"] != 0 || results[1]["index"] != 1 || results[2]["index"] != 2 {
		t.Fatalf("unexpected result order: %#v", results)
	}
	if results[1]["error"] != "boom-1" {
		t.Fatalf("results[1].error = %v", results[1]["error"])
	}

	collected := output["collected"].(map[string]any)
	ids := collected["ids"].([]any)
	errorsCollected := collected["errors"].([]any)
	statuses := collected["status"].([]any)
	if fmt.Sprint(ids) != "[a <nil> c]" {
		t.Fatalf("ids = %#v", ids)
	}
	if fmt.Sprint(errorsCollected) != "[<nil> boom-1 <nil>]" {
		t.Fatalf("errors = %#v", errorsCollected)
	}
	if fmt.Sprint(statuses) != "[ok-0 <nil> ok-2]" {
		t.Fatalf("statuses = %#v", statuses)
	}
}
