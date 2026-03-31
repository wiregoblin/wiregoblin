package model

import "testing"

func TestValidateWorkflowDoesNotMutateSteps(t *testing.T) {
	t.Parallel()

	definition := &Workflow{
		Name: "demo",
		Steps: []Step{{
			Type: BlockType("http"),
		}},
		OnErrorSteps: []Step{{
			Type: BlockType("telegram"),
		}},
	}

	if err := ValidateWorkflow(definition); err != nil {
		t.Fatalf("ValidateWorkflow() error = %v", err)
	}
	if definition.Steps[0].Name != "" {
		t.Fatalf("Steps[0].Name = %q, want empty", definition.Steps[0].Name)
	}
	if definition.OnErrorSteps[0].Name != "" {
		t.Fatalf("OnErrorSteps[0].Name = %q, want empty", definition.OnErrorSteps[0].Name)
	}
}

func TestWorkflowDefaultedCopyKeepsOriginalUntouched(t *testing.T) {
	t.Parallel()

	original := &Workflow{
		Name: "demo",
		Steps: []Step{{
			Type: BlockType("http"),
		}},
	}

	copyWorkflow := original.DefaultedCopy()
	if copyWorkflow.Steps[0].Name != "Step 1" {
		t.Fatalf("copyWorkflow.Steps[0].Name = %q, want %q", copyWorkflow.Steps[0].Name, "Step 1")
	}
	if original.Steps[0].Name != "" {
		t.Fatalf("original.Steps[0].Name = %q, want empty", original.Steps[0].Name)
	}
}
