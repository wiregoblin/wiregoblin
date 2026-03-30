package engine

import (
	"fmt"

	"github.com/wiregoblin/wiregoblin/internal/block"
)

// Registry stores the available workflow blocks.
type Registry struct {
	blockMap map[string]block.Block
}

// NewRegistry creates an empty block registry.
func NewRegistry() *Registry {
	return &Registry{
		blockMap: map[string]block.Block{},
	}
}

// Register adds one workflow block implementation.
func (r *Registry) Register(blk block.Block) {
	if blk == nil {
		return
	}
	r.blockMap[blk.Type()] = blk
}

// MustGet returns a block by type or an error if it is not registered.
func (r *Registry) MustGet(blockType string) (block.Block, error) {
	blk, ok := r.blockMap[blockType]
	if !ok {
		return nil, fmt.Errorf("unknown block type %q", blockType)
	}
	return blk, nil
}

// All returns all registered block implementations.
func (r *Registry) All() []block.Block {
	result := make([]block.Block, 0, len(r.blockMap))
	for _, blk := range r.blockMap {
		result = append(result, blk)
	}
	return result
}

// Close calls Close() on any block that implements blocks.Closer.
func (r *Registry) Close() {
	for _, blk := range r.blockMap {
		if closer, ok := blk.(block.Closer); ok {
			closer.Close()
		}
	}
}
