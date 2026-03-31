package parallel

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/model"
)

func TestValidateRejectsNestedAssignments(t *testing.T) {
	err := New().Validate(model.Step{
		Name: "fanout",
		Config: map[string]any{
			"blocks": []any{
				map[string]any{
					"id":   "fetch_user",
					"type": "http",
					"assign": map[string]any{
						"$user": "body",
					},
				},
			},
		},
	})
	if err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
	if got := err.Error(); got != "parallel blocks do not support nested runtime assignments; use collect instead" {
		t.Fatalf("error = %q", got)
	}
}

func TestExecuteCollectsResultsFromHeterogeneousBlocks(t *testing.T) {
	runCtx := &block.RunContext{
		Variables: map[string]string{"shared": "seed"},
		ExecuteIsolatedStep: func(
			_ context.Context,
			isolated *block.RunContext,
			step model.Step,
		) (*block.Result, error) {
			if got := isolated.Variables["shared"]; got != "seed" {
				t.Fatalf("shared = %q, want seed", got)
			}
			switch step.ID {
			case "slow":
				time.Sleep(20 * time.Millisecond)
				return &block.Result{
					Output: map[string]any{"body": map[string]any{"name": "Alice"}},
				}, nil
			case "fast":
				return &block.Result{
					Exports: map[string]string{"statusCode": "200"},
				}, nil
			default:
				return nil, errors.New("unexpected step")
			}
		},
	}

	result, err := New().Execute(context.Background(), runCtx, model.Step{
		Name: "fanout",
		Config: map[string]any{
			"blocks": []any{
				map[string]any{"id": "slow", "type": "http"},
				map[string]any{"id": "fast", "type": "redis"},
			},
			"collect": map[string]any{
				"$user_name": "slow.output.body.name",
				"$status":    "fast.exports.statusCode",
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := result.Output.(map[string]any)
	results := output["results"].(map[string]any)
	if _, ok := results["slow"]; !ok {
		t.Fatalf("results = %#v", results)
	}
	if _, ok := results["fast"]; !ok {
		t.Fatalf("results = %#v", results)
	}

	collected := output["collected"].(map[string]any)
	if got := collected["user_name"]; got != "Alice" {
		t.Fatalf("user_name = %v", got)
	}
	if got := collected["status"]; got != "200" {
		t.Fatalf("status = %v", got)
	}
	if got := result.Exports["$user_name"]; got != "Alice" {
		t.Fatalf("exports[$user_name] = %q", got)
	}
	if got := result.Exports["$status"]; got != "200" {
		t.Fatalf("exports[$status] = %q", got)
	}
	if got := runCtx.Variables["shared"]; got != "seed" {
		t.Fatalf("shared runtime mutated: %q", got)
	}
}

func TestExecuteAggregatesBranchFailures(t *testing.T) {
	runCtx := &block.RunContext{
		ExecuteIsolatedStep: func(
			_ context.Context,
			_ *block.RunContext,
			step model.Step,
		) (*block.Result, error) {
			if step.ID == "broken" {
				return &block.Result{Output: map[string]any{"kind": "broken"}}, errors.New("boom")
			}
			return &block.Result{Output: map[string]any{"kind": "ok"}}, nil
		},
	}

	result, err := New().Execute(context.Background(), runCtx, model.Step{
		Name: "fanout",
		Config: map[string]any{
			"blocks": []any{
				map[string]any{"id": "ok", "type": "http"},
				map[string]any{"id": "broken", "type": "redis"},
			},
		},
	})
	if err == nil {
		t.Fatal("Execute() error = nil, want aggregate error")
	}
	if got := err.Error(); got != "parallel failed: 1 of 2 branches failed: boom" {
		t.Fatalf("error = %q", got)
	}

	output := result.Output.(map[string]any)
	results := output["results"].(map[string]any)
	broken := results["broken"].(map[string]any)
	if got := broken["error"]; got != "boom" {
		t.Fatalf("broken.error = %v", got)
	}
}
