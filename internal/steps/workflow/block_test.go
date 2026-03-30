package workflowblock

import (
	"context"
	"testing"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/models"
)

func TestExecuteRunsNestedWorkflowFromRunContext(t *testing.T) {
	step := models.Step{
		Config: map[string]any{
			"target_workflow_uid": "child-workflow",
			"input_mapping": map[string]any{
				"RequestID": "req-42",
			},
		},
	}

	result, err := New().Execute(context.Background(), &block.RunContext{
		ExecuteWorkflow: func(_ context.Context, target string, inputs map[string]string) (*block.WorkflowRunResult, error) {
			if target != "child-workflow" {
				t.Fatalf("target = %q, want %q", target, "child-workflow")
			}
			if got := inputs["RequestID"]; got != "req-42" {
				t.Fatalf("input RequestID = %q, want %q", got, "req-42")
			}
			return &block.WorkflowRunResult{
				WorkflowUID: "wf-1",
				Workflow:    "Child Workflow",
				Variables:   map[string]string{"ResultID": "done"},
				Exports:     map[string]string{"ResultID": "done"},
			}, nil
		},
	}, step)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := result.Output.(map[string]any)
	if output["target_workflow_uid"] != "child-workflow" {
		t.Fatalf("target_workflow_uid = %v, want %q", output["target_workflow_uid"], "child-workflow")
	}
	if output["workflow"] != "Child Workflow" {
		t.Fatalf("workflow = %v, want %q", output["workflow"], "Child Workflow")
	}
	if result.Exports["ResultID"] != "done" {
		t.Fatalf("exports ResultID = %q, want %q", result.Exports["ResultID"], "done")
	}
}

func TestExecuteReturnsErrorWithoutWorkflowExecutor(t *testing.T) {
	step := models.Step{
		Config: map[string]any{
			"target_workflow_uid": "child-workflow",
		},
	}

	result, err := New().Execute(context.Background(), nil, step)
	if err == nil {
		t.Fatal("Execute() error = nil, want executor error")
	}
	if err.Error() != "workflow block requires runner workflow execution support" {
		t.Fatalf("error = %q", err.Error())
	}
	output := result.Output.(map[string]any)
	if output["target_workflow_uid"] != "child-workflow" {
		t.Fatalf("target_workflow_uid = %v, want %q", output["target_workflow_uid"], "child-workflow")
	}
}
