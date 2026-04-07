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
  - id: sample
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

	wf := workflowByID(project, "sample")
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

func TestParsePreservesAIConfigWithDefaults(t *testing.T) {
	project, err := parse([]byte(`
id: demo
name: Demo
ai:
  provider: Ollama
  base_url: http://127.0.0.1:11434
  model: llama3.1:8b
workflows:
  - id: sample
    blocks: []
`))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if project.Meta.AI == nil {
		t.Fatal("expected ai config to be parsed")
	}
	if !project.Meta.AI.Enabled {
		t.Fatal("expected ai.enabled to default to true")
	}
	if got := project.Meta.AI.Provider; got != "ollama" {
		t.Fatalf("provider = %q, want %q", got, "ollama")
	}
	if got := project.Meta.AI.BaseURL; got != "http://127.0.0.1:11434" {
		t.Fatalf("base_url = %q, want %q", got, "http://127.0.0.1:11434")
	}
	if got := project.Meta.AI.Model; got != "llama3.1:8b" {
		t.Fatalf("model = %q, want %q", got, "llama3.1:8b")
	}
	if !project.Meta.AI.RedactSecrets {
		t.Fatal("expected ai.redact_secrets to default to true")
	}
}

func TestParseAllowsDisabledAIWithoutRuntimeFields(t *testing.T) {
	project, err := parse([]byte(`
id: demo
name: Demo
ai:
  enabled: false
workflows:
  - id: sample
    blocks: []
`))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if project.Meta.AI == nil {
		t.Fatal("expected ai config to be parsed")
	}
	if project.Meta.AI.Enabled {
		t.Fatal("expected ai.enabled to be false")
	}
}

func TestParseResolvesAIEnvReferences(t *testing.T) {
	t.Setenv("WG_AI_PROVIDER", "openai_compatible")
	t.Setenv("WG_AI_BASE_URL", "http://127.0.0.1:1234/v1")
	t.Setenv("WG_AI_MODEL", "qwen2.5-coder-7b-instruct")

	project, err := parse([]byte(`
id: demo
name: Demo
ai:
  provider: "${WG_AI_PROVIDER}"
  base_url: "${WG_AI_BASE_URL}"
  model: "${WG_AI_MODEL}"
workflows:
  - id: sample
    blocks: []
`))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if project.Meta.AI == nil {
		t.Fatal("expected ai config to be parsed")
	}
	if got := project.Meta.AI.Provider; got != "openai_compatible" {
		t.Fatalf("provider = %q, want %q", got, "openai_compatible")
	}
	if got := project.Meta.AI.BaseURL; got != "http://127.0.0.1:1234/v1" {
		t.Fatalf("base_url = %q, want %q", got, "http://127.0.0.1:1234/v1")
	}
	if got := project.Meta.AI.Model; got != "qwen2.5-coder-7b-instruct" {
		t.Fatalf("model = %q, want %q", got, "qwen2.5-coder-7b-instruct")
	}
}

func TestParseRejectsInvalidAIProvider(t *testing.T) {
	_, err := parse([]byte(`
id: demo
name: Demo
ai:
  provider: lmstudio
  base_url: http://127.0.0.1:1234/v1
  model: qwen2.5
workflows:
  - id: sample
    blocks: []
`))
	if err == nil {
		t.Fatal("expected parse to fail for invalid provider")
	}
}

func TestParseRejectsEnabledAIWithoutRequiredFields(t *testing.T) {
	_, err := parse([]byte(`
id: demo
name: Demo
ai:
  enabled: true
  provider: ollama
workflows:
  - id: sample
    blocks: []
`))
	if err == nil {
		t.Fatal("expected parse to fail when ai fields are missing")
	}
}

func TestParseRejectsLegacyWorkflowMapFormat(t *testing.T) {
	_, err := parse([]byte(`
id: demo
name: Demo
workflows:
  sample:
    name: Sample
    blocks: []
`))
	if err == nil {
		t.Fatal("expected parse to fail for legacy workflows map format")
	}
	if got := err.Error(); got != "parse yaml: "+legacyWorkflowMapFormatError {
		t.Fatalf("error = %q", got)
	}
}

func TestParsePreservesWorkflowTargetID(t *testing.T) {
	project, err := parse([]byte(`
id: demo
name: Demo
workflows:
  - id: child
    blocks: []
  - id: parent
    blocks:
      - id: nested
        type: workflow
        target_workflow_id: child
`))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	target, _ := workflowByID(project, "parent").Steps[0].Config["target_workflow_id"].(string)
	if target != "child" {
		t.Fatalf("target_workflow_id = %q, want %q", target, "child")
	}
}

func TestParseWorkflowDisableRunDefaultsToFalse(t *testing.T) {
	project, err := parse([]byte(`
id: demo
name: Demo
workflows:
  - id: sample
    blocks: []
`))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if workflowByID(project, "sample").DisableRun {
		t.Fatal("expected disable_run to default to false")
	}
}

func TestParseWorkflowDisableRunCanBeEnabled(t *testing.T) {
	project, err := parse([]byte(`
id: demo
name: Demo
workflows:
  - id: sample
    disable_run: true
    blocks: []
`))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if !workflowByID(project, "sample").DisableRun {
		t.Fatal("expected disable_run to be true")
	}
}

func TestParsePreservesAssignConfig(t *testing.T) {
	project, err := parse([]byte(`
id: demo
name: Demo
workflows:
  - id: sample
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

	config := workflowByID(project, "sample").Steps[0].Config
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
  - id: sample
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

	steps := workflowByID(project, "sample").Steps
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
  - id: sample
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

	condition := workflowByID(project, "sample").Steps[0].Condition
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
  - id: sample
    blocks:
      - id: notify
        type: telegram
        continue_on_error: true
`))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if !workflowByID(project, "sample").Steps[0].ContinueOnError {
		t.Fatal("expected continue_on_error to be parsed")
	}
}

func TestParsePreservesWorkflowTimeoutSeconds(t *testing.T) {
	project, err := parse([]byte(`
id: demo
name: Demo
workflows:
  - id: sample
    timeout_seconds: 30
    blocks: []
`))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if got := workflowByID(project, "sample").TimeoutSeconds; got != 30 {
		t.Fatalf("timeout_seconds = %d, want 30", got)
	}
}

func TestParsePreservesWorkflowOutputs(t *testing.T) {
	project, err := parse([]byte(`
id: demo
name: Demo
workflows:
  - id: sample
    outputs:
      child_run_label: "$child_run_label"
    blocks: []
`))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	outputs := workflowByID(project, "sample").Outputs
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

func workflowByID(def *model.Definition, id string) *model.Workflow {
	if def == nil {
		return nil
	}
	return def.WorkflowByID[id]
}
