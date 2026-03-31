// Package workflowblock implements nested workflow execution steps.
package workflowblock

import (
	"context"
	"fmt"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/model"
)

const blockType = "workflow"

// Block implements nested workflow execution.
type Block struct{}

// New creates the workflow block.
func New() *Block {
	return &Block{}
}

// Type returns the workflow block identifier.
func (b *Block) Type() model.BlockType {
	return blockType
}

// ReferencePolicy describes which fields accept @ references and runtime variables.
func (b *Block) ReferencePolicy() []block.ReferencePolicy {
	return []block.ReferencePolicy{
		{Field: "inputs", Constants: true, Variables: true, InlineOnly: true},
	}
}

// SupportsResponseMapping reports whether the block exposes structured nested workflow output.
func (b *Block) SupportsResponseMapping() bool {
	return true
}

// Validate checks the minimal nested workflow fields.
func (b *Block) Validate(step model.Step) error {
	config := decodeConfig(step)
	if config.TargetWorkflowID == "" {
		return fmt.Errorf("target workflow is required")
	}
	return nil
}

// Execute runs the target workflow through the runner callback stored in RunContext.
func (b *Block) Execute(ctx context.Context, runCtx *block.RunContext, step model.Step) (*block.Result, error) {
	config := decodeConfig(step)
	result := &block.Result{
		Output: map[string]any{
			"target_workflow_id": config.TargetWorkflowID,
			"inputs":             config.Inputs,
		},
	}
	if runCtx == nil || runCtx.ExecuteWorkflow == nil {
		return result, fmt.Errorf("workflow block requires runner workflow execution support")
	}

	nested, err := runCtx.ExecuteWorkflow(ctx, config.TargetWorkflowID, config.Inputs)
	if nested != nil {
		rawExports := nested.RawExports
		if rawExports == nil {
			rawExports = nested.Exports
		}
		result.Output = map[string]any{
			"target_workflow_id": config.TargetWorkflowID,
			"workflow_id":        nested.WorkflowID,
			"workflow":           nested.Workflow,
			"inputs":             config.Inputs,
			"steps":              nested.Steps,
			"variables":          nested.Variables,
			"secret_variables":   nested.SecretVariables,
			"outputs":            nested.Exports,
		}
		result.Exports = rawExports
	}
	if err != nil {
		return result, err
	}
	return result, nil
}
