// Package setvars implements the workflow step that assigns runtime variables.
package setvars

import (
	"context"
	"fmt"
	"strings"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/model"
)

const blockType = "setvars"

// Block implements a variable assignment step.
type Block struct{}

// New creates a set variable workflow block.
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
		{Field: "set", Constants: true, Variables: true, InlineOnly: true},
	}
}

// Validate checks that at least one variable is specified.
func (b *Block) Validate(step model.Step) error {
	config := decodeConfig(step)
	if len(config.Set) == 0 {
		return fmt.Errorf("set variable: at least one set entry is required")
	}
	for variable := range config.Set {
		if strings.TrimSpace(variable) == "" {
			return fmt.Errorf("set variable: target variable is required")
		}
	}
	return nil
}

// Execute exports the already-resolved assignment value to the target variable.
func (b *Block) Execute(
	_ context.Context,
	_ *block.RunContext,
	step model.Step,
) (*block.Result, error) {
	config := decodeConfig(step)
	outputSet := make(map[string]string, len(config.Set))
	exports := make(map[string]string, len(config.Set))

	for variable, value := range config.Set {
		outputSet[variable] = value
		exports[variable] = value
	}

	return &block.Result{
		Output: map[string]any{
			"set": outputSet,
		},
		Exports: exports,
	}, nil
}
