// Package delay implements delay workflow steps.
package delay

import (
	"context"
	"fmt"
	"time"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/model"
)

const blockType = "delay"

// Block implements a configurable pause step.
type Block struct{}

// New creates a delay workflow block.
func New() *Block {
	return &Block{}
}

// Type returns the block type identifier.
func (b *Block) Type() model.BlockType {
	return blockType
}

// Validate checks that the duration is positive.
func (b *Block) Validate(step model.Step) error {
	config := decodeConfig(step)
	if config.Milliseconds <= 0 {
		return fmt.Errorf("delay milliseconds must be greater than zero")
	}
	return nil
}

// Execute pauses execution for the configured duration, respecting context cancellation.
func (b *Block) Execute(
	ctx context.Context,
	_ *block.RunContext,
	step model.Step,
) (*block.Result, error) {
	config := decodeConfig(step)

	timer := time.NewTimer(time.Duration(config.Milliseconds) * time.Millisecond)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
	}

	return &block.Result{
		Output: map[string]any{
			"milliseconds": config.Milliseconds,
		},
	}, nil
}
