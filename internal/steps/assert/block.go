// Package assert implements assertion workflow steps.
package assert

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/conditions"
	"github.com/wiregoblin/wiregoblin/internal/models"
	"github.com/wiregoblin/wiregoblin/internal/steps"
)

// Block implements a simple assertion step.
type Block struct{}

// New creates an assertion workflow block.
func New() *Block {
	return &Block{}
}

// Type returns the block type identifier.
func (b *Block) Type() string {
	return steps.BlockTypeAssert
}

// ReferencePolicy describes which fields accept constants and runtime variables.
func (b *Block) ReferencePolicy() []block.ReferencePolicy {
	return []block.ReferencePolicy{
		{Field: "expected", Constants: true, Variables: true, InlineOnly: true},
	}
}

// Validate checks the minimal assert fields.
func (b *Block) Validate(step models.Step) error {
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
	step models.Step,
) (*block.Result, error) {
	config := decodeConfig(step)
	actual := ""
	if runCtx != nil {
		actual = runCtx.Variables[config.Variable]
	}

	result, err := conditions.Evaluate(actual, config.Operator, config.Expected)
	if err != nil {
		return nil, err
	}

	output := map[string]any{
		"variable": config.Variable,
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
