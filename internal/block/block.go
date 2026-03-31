package block

import (
	"context"

	"github.com/wiregoblin/wiregoblin/internal/model"
)

// Block is the common runtime contract for workflow blocks.
type Block interface {
	Type() model.BlockType
	Validate(step model.Step) error
	Execute(ctx context.Context, runCtx *RunContext, step model.Step) (*Result, error)
}
