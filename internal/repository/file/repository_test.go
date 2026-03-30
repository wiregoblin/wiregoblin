package filerepository

import (
	"testing"

	"github.com/google/uuid"

	"github.com/wiregoblin/wiregoblin/internal/models"
)

func TestParsePreservesVariableInitialValues(t *testing.T) {
	project, err := parse([]byte(`
id: demo
name: Demo
variables:
  projectName: alpha
  retries: 3
workflows:
  sample:
    variables:
      workflowName: beta
      enabled: true
    blocks: {}
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

	wf := project.Workflows["sample"]
	if got := entryValue(wf.Variables, "workflowName"); got != "beta" {
		t.Fatalf("expected workflowName=beta, got %q", got)
	}
	if got := entryValue(wf.Variables, "enabled"); got != "true" {
		t.Fatalf("expected enabled=true, got %q", got)
	}
}

func TestParseNormalizesWorkflowTargetToWorkflowUUID(t *testing.T) {
	project, err := parse([]byte(`
id: demo
name: Demo
workflows:
  child:
    blocks: {}
  parent:
    blocks:
      nested:
        type: workflow
        target_workflow_uid: child
`))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	target, _ := project.Workflows["parent"].Steps[0].Config["target_workflow_uid"].(string)
	if target != project.Workflows["child"].ID.String() {
		t.Fatalf(
			"target_workflow_uid = %q, want %q",
			target,
			project.Workflows["child"].ID.String(),
		)
	}
	if _, err := uuid.Parse(target); err != nil {
		t.Fatalf("target_workflow_uid is not a UUID: %v", err)
	}
}

func entryValue(entries []models.Entry, key string) string {
	for _, entry := range entries {
		if entry.Key == key {
			return entry.Value
		}
	}
	return ""
}
