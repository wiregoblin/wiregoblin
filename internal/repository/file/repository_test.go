package filerepository

import (
	"testing"

	"github.com/wiregoblin/wiregoblin/internal/model"
)

func TestParsePreservesVariableInitialValues(t *testing.T) {
	project, err := parse([]byte(`
id: demo
name: Demo
variables:
  projectName: alpha
  retries: 3
secret_variables:
  sessionToken: hidden
workflows:
  sample:
    variables:
      workflowName: beta
      enabled: true
    secret_variables:
      workflowToken: masked
    blocks: []
`))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if got := entryValue(project.Meta.Variables, "projectName"); got != "alpha" {
		t.Fatalf("expected projectName=alpha, got %q", got)
	}
	if got := entryValue(project.Meta.Variables, "retries"); got != "3" {
		t.Fatalf("expected retries=3, got %q", got)
	}
	if got := entryValue(project.Meta.SecretVariables, "sessionToken"); got != "hidden" {
		t.Fatalf("expected sessionToken=hidden, got %q", got)
	}

	wf := project.Workflows["sample"]
	if got := entryValue(wf.Variables, "workflowName"); got != "beta" {
		t.Fatalf("expected workflowName=beta, got %q", got)
	}
	if got := entryValue(wf.Variables, "enabled"); got != "true" {
		t.Fatalf("expected enabled=true, got %q", got)
	}
	if got := entryValue(wf.SecretVariables, "workflowToken"); got != "masked" {
		t.Fatalf("expected workflowToken=masked, got %q", got)
	}
}

func TestParsePreservesWorkflowTargetID(t *testing.T) {
	project, err := parse([]byte(`
id: demo
name: Demo
workflows:
  child:
    blocks: []
  parent:
    blocks:
      - id: nested
        type: workflow
        target_workflow_id: child
`))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	target, _ := project.Workflows["parent"].Steps[0].Config["target_workflow_id"].(string)
	if target != "child" {
		t.Fatalf("target_workflow_id = %q, want %q", target, "child")
	}
}

func TestParsePreservesAssignConfig(t *testing.T) {
	project, err := parse([]byte(`
id: demo
name: Demo
workflows:
  sample:
    blocks:
      - id: step
        type: http
        assign:
          $user_id: body.id
          $http_status: outputs.statusCode
`))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	config := project.Workflows["sample"].Steps[0].Config
	assign, ok := config["assign"].([]any)
	if !ok {
		t.Fatalf("expected normalized assign slice, got %#v", config["assign"])
	}
	if len(assign) != 2 {
		t.Fatalf("expected 2 normalized assign entries, got %d", len(assign))
	}
}

func TestParsePreservesBlockSequenceOrder(t *testing.T) {
	project, err := parse([]byte(`
id: demo
name: Demo
workflows:
  sample:
    blocks:
      - id: second
        type: assert
        variable: step
        operator: "="
        expected: "2"
      - id: first
        type: assert
        variable: step
        operator: "="
        expected: "1"
`))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	steps := project.Workflows["sample"].Steps
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}
	if steps[0].Name != "second" || steps[1].Name != "first" {
		t.Fatalf("unexpected step order: %#v", []string{steps[0].Name, steps[1].Name})
	}
}

func TestParsePreservesStepCondition(t *testing.T) {
	project, err := parse([]byte(`
id: demo
name: Demo
workflows:
  sample:
    blocks:
      - id: step
        type: log
        condition:
          variable: "$status_!Retry.Attempt"
          operator: "="
          expected: "ready"
`))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	condition := project.Workflows["sample"].Steps[0].Condition
	if condition == nil {
		t.Fatal("expected step condition to be parsed")
	}
	if condition.Variable != "$status_!Retry.Attempt" {
		t.Fatalf("condition variable = %q, want %q", condition.Variable, "$status_!Retry.Attempt")
	}
	if condition.Operator != "=" {
		t.Fatalf("condition operator = %q, want %q", condition.Operator, "=")
	}
	if condition.Expected != "ready" {
		t.Fatalf("condition expected = %q, want %q", condition.Expected, "ready")
	}
}

func TestParsePreservesContinueOnError(t *testing.T) {
	project, err := parse([]byte(`
id: demo
name: Demo
workflows:
  sample:
    blocks:
      - id: notify
        type: telegram
        continue_on_error: true
`))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if !project.Workflows["sample"].Steps[0].ContinueOnError {
		t.Fatal("expected continue_on_error to be parsed")
	}
}

func TestParsePreservesWorkflowTimeoutSeconds(t *testing.T) {
	project, err := parse([]byte(`
id: demo
name: Demo
workflows:
  sample:
    timeout_seconds: 30
    blocks: []
`))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if got := project.Workflows["sample"].TimeoutSeconds; got != 30 {
		t.Fatalf("timeout_seconds = %d, want 30", got)
	}
}

func TestParsePreservesWorkflowOutputs(t *testing.T) {
	project, err := parse([]byte(`
id: demo
name: Demo
workflows:
  sample:
    outputs:
      child_run_label: "$child_run_label"
    blocks: []
`))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	outputs := project.Workflows["sample"].Outputs
	if got := outputs["child_run_label"]; got != "$child_run_label" {
		t.Fatalf("outputs[child_run_label] = %q, want %q", got, "$child_run_label")
	}
}

func entryValue(entries []model.Entry, key string) string {
	for _, entry := range entries {
		if entry.Key == key {
			return entry.Value
		}
	}
	return ""
}
