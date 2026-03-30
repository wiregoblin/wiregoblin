// Package workflowblock implements nested workflow execution steps.
package workflowblock

import (
	"context"
	"fmt"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/models"
	"github.com/wiregoblin/wiregoblin/internal/steps"
)

// Block implements nested workflow execution.
type Block struct{}

// New creates the workflow block.
func New() *Block {
	return &Block{}
}

// Type returns the workflow block identifier.
func (b *Block) Type() string {
	return steps.BlockTypeWorkflow
}

// ReferencePolicy describes which fields accept constants and runtime variables.
func (b *Block) ReferencePolicy() []block.ReferencePolicy {
	return []block.ReferencePolicy{
		{Field: "input_mapping", Constants: true, Variables: true, InlineOnly: true},
	}
}

// SupportsResponseMapping reports whether the block exposes structured nested workflow output.
func (b *Block) SupportsResponseMapping() bool {
	return true
}

// Validate checks the minimal nested workflow fields.
func (b *Block) Validate(step models.Step) error {
	config := decodeConfig(step)
	if config.TargetWorkflowUID == "" {
		return fmt.Errorf("target workflow is required")
	}
	return nil
}

// Execute runs the target workflow through the runner callback stored in RunContext.
func (b *Block) Execute(ctx context.Context, runCtx *block.RunContext, step models.Step) (*block.Result, error) {
	config := decodeConfig(step)
	result := &block.Result{
		Output: map[string]any{
			"target_workflow_uid": config.TargetWorkflowUID,
			"input_mapping":       config.InputMapping,
		},
	}
	if runCtx == nil || runCtx.ExecuteWorkflow == nil {
		return result, fmt.Errorf("workflow block requires runner workflow execution support")
	}

	nested, err := runCtx.ExecuteWorkflow(ctx, config.TargetWorkflowUID, config.InputMapping)
	if nested != nil {
		rawExports := nested.RawExports
		if rawExports == nil {
			rawExports = nested.Exports
		}
		result.Output = map[string]any{
			"target_workflow_uid": config.TargetWorkflowUID,
			"workflow_uid":        nested.WorkflowUID,
			"workflow":            nested.Workflow,
			"input_mapping":       config.InputMapping,
			"steps":               nested.Steps,
			"variables":           nested.Variables,
			"exports":             nested.Exports,
		}
		result.Exports = rawExports
	}
	if err != nil {
		return result, err
	}
	return result, nil
}
