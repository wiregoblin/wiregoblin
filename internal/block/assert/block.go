// Package assert implements assertion workflow steps.
package assert

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/condition"
	"github.com/wiregoblin/wiregoblin/internal/model"
)

const blockType = "assert"

// Block implements a simple assertion step.
type Block struct{}

// New creates an assertion workflow block.
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

// Validate checks the minimal assert fields.
func (b *Block) Validate(step model.Step) error {
	config := decodeConfig(step)
	if config.Variable == "" {
		return fmt.Errorf("assert variable is required")
	}
	if config.Operator == "" {
		return fmt.Errorf("assert operator is required")
	}
	return nil
}

// Execute compares the current variable value with the expected one.
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

	result, err := condition.Evaluate(actual, config.Operator, config.Expected)
	if err != nil {
		return nil, err
	}

	output := map[string]any{
		"variable": variableName,
		"actual":   actual,
		"operator": config.Operator,
		"expected": config.Expected,
		"passed":   result,
	}
	if !result {
		return &block.Result{Output: output}, errors.New(fallbackErrorMessage(config.ErrorMessage))
	}

	return &block.Result{Output: output}, nil
}

func fallbackErrorMessage(message string) string {
	if strings.TrimSpace(message) == "" {
		return "Assertion failed"
	}
	return strings.TrimSpace(message)
}
