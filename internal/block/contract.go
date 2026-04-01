package block

import (
	"context"

	"github.com/wiregoblin/wiregoblin/internal/model"
)

// ReferencePolicy describes which references are allowed for a block field.
// Built-ins use the ! namespace and are allowed anywhere constants or variables are allowed.
type ReferencePolicy struct {
	Field      string
	Constants  bool
	Variables  bool
	InlineOnly bool
}

// RunContext contains project-scoped data and previous step outputs.
type RunContext struct {
	ProjectID       string
	Constants       map[string]string
	Secrets         map[string]string
	SecretVariables map[string]string
	Variables       map[string]string
	Builtins        map[string]string
	StepResults     map[string]any
	ExecuteWorkflow func(
		ctx context.Context,
		targetWorkflowID string,
		inputs map[string]string,
	) (*WorkflowRunResult, error)
	ExecuteStep func(
		ctx context.Context,
		step model.Step,
	) (*Result, error)
	ExecuteIsolatedStep func(
		ctx context.Context,
		runCtx *RunContext,
		step model.Step,
	) (*Result, error)
}

// Jump instructs the runner to continue execution from a target step.
type Jump struct {
	TargetStepID string
	WaitSeconds  int
}

// Result is the output of one executed workflow block.
type Result struct {
	Output  any               `json:"output,omitempty"`
	Exports map[string]string `json:"exports,omitempty"`
	Jump    *Jump             `json:"-"`
}

// WorkflowStepResult is a block-safe representation of one nested workflow step result.
type WorkflowStepResult struct {
	Index  int    `json:"index"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Status string `json:"status"`
	Output any    `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

// WorkflowRunResult describes the outcome of a nested workflow execution.
type WorkflowRunResult struct {
	WorkflowID      string               `json:"workflow_id"`
	Steps           []WorkflowStepResult `json:"steps,omitempty"`
	Variables       map[string]string    `json:"variables,omitempty"`
	SecretVariables map[string]string    `json:"secret_variables,omitempty"`
	Exports         map[string]string    `json:"exports,omitempty"`
	RawExports      map[string]string    `json:"-"`
}
