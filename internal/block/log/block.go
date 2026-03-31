// Package logblock implements log-emitting workflow steps.
package logblock

import (
	"context"
	"fmt"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/model"
)

const blockType = "log"

// Block emits a log-style message into the step output.
type Block struct{}

// New creates a log workflow block.
func New() *Block {
	return &Block{}
}

// Type returns the block type identifier.
func (b *Block) Type() model.BlockType {
	return blockType
}

// ReferencePolicy describes which fields accept @, $, and ! references.
func (b *Block) ReferencePolicy() []block.ReferencePolicy {
	return []block.ReferencePolicy{
		{Field: "message", Constants: true, Variables: true, InlineOnly: true},
		{Field: "level", Constants: true, Variables: true, InlineOnly: true},
	}
}

// Validate checks the minimal log fields.
func (b *Block) Validate(step model.Step) error {
	config := decodeConfig(step)
	if config.Message == "" {
		return fmt.Errorf("log message is required")
	}
	if _, err := normalizeLevel(config.Level); err != nil {
		return err
	}
	return nil
}

// Execute returns the resolved log payload without mutating runtime state.
func (b *Block) Execute(
	_ context.Context,
	_ *block.RunContext,
	step model.Step,
) (*block.Result, error) {
	config := decodeConfig(step)
	level, _ := normalizeLevel(config.Level)
	return &block.Result{
		Output: map[string]any{
			"message": config.Message,
			"level":   level,
		},
	}, nil
}
