// Package setvars implements the workflow step that assigns runtime variables.
package setvars

import (
	"context"
	"fmt"
	"strings"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/models"
	"github.com/wiregoblin/wiregoblin/internal/steps"
)

// Block implements a variable assignment step.
type Block struct{}

// New creates a set variable workflow block.
func New() *Block {
	return &Block{}
}

// Type returns the block type identifier.
func (b *Block) Type() string {
	return steps.BlockTypeSetVars
}

// ReferencePolicy describes which fields accept constants and runtime variables.
func (b *Block) ReferencePolicy() []block.ReferencePolicy {
	return []block.ReferencePolicy{
		{Field: "assignments", Constants: true, Variables: true, InlineOnly: true},
	}
}

// Validate checks that at least one assignment is specified.
func (b *Block) Validate(step models.Step) error {
	config := decodeConfig(step)
	if len(config.Assignments) == 0 {
		return fmt.Errorf("set variable: at least one assignment is required")
	}
	for _, assignment := range config.Assignments {
		if strings.TrimSpace(assignment.Variable) == "" {
			return fmt.Errorf("set variable: target variable is required")
		}
	}
	return nil
}

// Execute exports the already-resolved assignment value to the target variable.
func (b *Block) Execute(
	_ context.Context,
	_ *block.RunContext,
	step models.Step,
) (*block.Result, error) {
	config := decodeConfig(step)
	outputAssignments := make([]map[string]string, 0, len(config.Assignments))
	exports := make(map[string]string, len(config.Assignments))

	for _, assignment := range config.Assignments {
		outputAssignments = append(outputAssignments, map[string]string{
			"variable": assignment.Variable,
			"value":    assignment.Value,
		})
		exports[assignment.Variable] = assignment.Value
	}

	return &block.Result{
		Output: map[string]any{
			"assignments": outputAssignments,
		},
		Exports: exports,
	}, nil
}
