package block

import (
	"context"

	"github.com/google/uuid"

	"github.com/wiregoblin/wiregoblin/internal/models"
)

// ReferencePolicy describes which references are allowed for a block field.
type ReferencePolicy struct {
	Field      string
	Constants  bool
	Variables  bool
	InlineOnly bool
}

// RunContext contains project-scoped data and previous step outputs.
type RunContext struct {
	ProjectID       uuid.UUID
	Constants       map[string]string
	Secrets         map[string]string
	Variables       map[string]string
	StepResults     map[string]any
	ExecuteWorkflow func(
		ctx context.Context,
		targetWorkflowUID string,
		inputs map[string]string,
	) (*WorkflowRunResult, error)
}

// Jump instructs the runner to continue execution from a target step.
type Jump struct {
	TargetStepUID string
	WaitSeconds   int
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
	WorkflowUID string               `json:"workflow_uid"`
	Workflow    string               `json:"workflow"`
	Steps       []WorkflowStepResult `json:"steps,omitempty"`
	Variables   map[string]string    `json:"variables,omitempty"`
	Exports     map[string]string    `json:"exports,omitempty"`
	RawExports  map[string]string    `json:"-"`
}

// Block is the common runtime contract for workflow blocks.
type Block interface {
	Type() string
	Validate(step models.Step) error
	Execute(ctx context.Context, runCtx *RunContext, step models.Step) (*Result, error)
}
