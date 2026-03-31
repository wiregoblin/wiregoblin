// Package gotoblock implements conditional jump workflow steps.
package gotoblock

import (
	"context"
	"fmt"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/condition"
	"github.com/wiregoblin/wiregoblin/internal/model"
)

const blockType = "goto"

// Block implements a conditional jump step.
type Block struct{}

// New creates a goto workflow block.
func New() *Block {
	return &Block{}
}

// Type returns the block type identifier.
func (b *Block) Type() model.BlockType {
	return blockType
}

// ReferencePolicy describes which fields accept @ references and runtime variables.
func (b *Block) ReferencePolicy() []block.ReferencePolicy {
	return []block.ReferencePolicy{
		{Field: "expected", Constants: true, Variables: true, InlineOnly: true},
	}
}

// Validate checks the minimal goto fields.
func (b *Block) Validate(step model.Step) error {
	config := decodeConfig(step)
	if config.Variable == "" {
		return fmt.Errorf("goto variable is required")
	}
	if config.Operator == "" {
		return fmt.Errorf("goto operator is required")
	}
	if config.TargetStepID == "" {
		return fmt.Errorf("goto target is required")
	}
	return nil
}

// Execute evaluates the condition and returns the jump target when matched.
func (b *Block) Execute(
	_ context.Context,
	runCtx *block.RunContext,
	step model.Step,
) (*block.Result, error) {
	config := decodeConfig(step)
	variableName, actual, _ := block.ResolveVariableExpression(runCtx, config.Variable)
	if variableName == "" {
		variableName = config.Variable
	}

	matched, err := condition.Evaluate(actual, config.Operator, config.Expected)
	if err != nil {
		return nil, err
	}

	output := map[string]any{
		"variable":       variableName,
		"actual":         actual,
		"operator":       config.Operator,
		"expected":       config.Expected,
		"matched":        matched,
		"target_step_id": config.TargetStepID,
		"wait_seconds":   config.WaitSeconds,
	}
	result := &block.Result{Output: output}
	if matched {
		result.Jump = &block.Jump{
			TargetStepID: config.TargetStepID,
			WaitSeconds:  config.WaitSeconds,
		}
	}
	return result, nil
}
