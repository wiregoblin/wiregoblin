package engine

import (
	"fmt"
	"sync"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/model"
)

// Registry stores the available workflow blocks.
type Registry struct {
	mu       sync.RWMutex
	blockMap map[model.BlockType]block.Block
}

// NewRegistry creates an empty block registry.
func NewRegistry() *Registry {
	return &Registry{
		blockMap: map[model.BlockType]block.Block{},
	}
}

// Register adds one workflow block implementation.
func (r *Registry) Register(blk block.Block) {
	if blk == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.blockMap[blk.Type()] = blk
}

// Get returns a block by type or an error if it is not registered.
func (r *Registry) Get(blockType model.BlockType) (block.Block, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	blk, ok := r.blockMap[blockType]
	if !ok {
		return nil, fmt.Errorf("unknown block type %q", blockType)
	}
	return blk, nil
}

// All returns all registered block implementations.
func (r *Registry) All() []block.Block {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]block.Block, 0, len(r.blockMap))
	for _, blk := range r.blockMap {
		result = append(result, blk)
	}
	return result
}

// Close calls Close() on any block that implements blocks.Closer.
func (r *Registry) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, blk := range r.blockMap {
		if closer, ok := blk.(block.Closer); ok {
			closer.Close()
		}
	}
}
