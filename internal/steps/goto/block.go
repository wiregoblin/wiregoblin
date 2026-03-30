// Package gotoblock implements conditional jump workflow steps.
package gotoblock

import (
	"context"
	"fmt"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/conditions"
	"github.com/wiregoblin/wiregoblin/internal/models"
	"github.com/wiregoblin/wiregoblin/internal/steps"
)

// Block implements a conditional jump step.
type Block struct{}

// New creates a goto workflow block.
func New() *Block {
	return &Block{}
}

// Type returns the block type identifier.
func (b *Block) Type() string {
	return steps.BlockTypeGoto
}

// ReferencePolicy describes which fields accept constants and runtime variables.
func (b *Block) ReferencePolicy() []block.ReferencePolicy {
	return []block.ReferencePolicy{
		{Field: "expected", Constants: true, Variables: true, InlineOnly: true},
	}
}

// Validate checks the minimal goto fields.
func (b *Block) Validate(step models.Step) error {
	config := decodeConfig(step)
	if config.Variable == "" {
		return fmt.Errorf("goto variable is required")
	}
	if config.Operator == "" {
		return fmt.Errorf("goto operator is required")
	}
	if config.TargetStepUID == "" {
		return fmt.Errorf("goto target is required")
	}
	return nil
}

// Execute evaluates the condition and returns the jump target when matched.
func (b *Block) Execute(
	_ context.Context,
	runCtx *block.RunContext,
	step models.Step,
) (*block.Result, error) {
	config := decodeConfig(step)
	actual := ""
	if runCtx != nil {
		actual = runCtx.Variables[config.Variable]
	}

	matched, err := conditions.Evaluate(actual, config.Operator, config.Expected)
	if err != nil {
		return nil, err
	}

	output := map[string]any{
		"variable":        config.Variable,
		"actual":          actual,
		"operator":        config.Operator,
		"expected":        config.Expected,
		"matched":         matched,
		"target_step_uid": config.TargetStepUID,
		"wait_seconds":    config.WaitSeconds,
	}
	result := &block.Result{Output: output}
	if matched {
		result.Jump = &block.Jump{
			TargetStepUID: config.TargetStepUID,
			WaitSeconds:   config.WaitSeconds,
		}
	}
	return result, nil
}
