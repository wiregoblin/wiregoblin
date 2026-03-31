// Package grpc implements gRPC workflow steps.
package grpc

import (
	"context"
	"fmt"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/model"
)

const blockType = "grpc"

// Block implements the workflow block contract for unary gRPC requests.
type Block struct {
	// invoke replaces the default Invoke implementation in tests.
	invoke func(ctx context.Context, config Config) (string, error)
}

// New creates a gRPC workflow block.
func New() *Block {
	return &Block{}
}

// Type returns the block type identifier.
func (b *Block) Type() model.BlockType {
	return blockType
}

// SupportsResponseMapping reports whether the block exposes response mapping.
func (b *Block) SupportsResponseMapping() bool {
	return true
}

// ReferencePolicy describes which fields accept @ references and runtime variables.
func (b *Block) ReferencePolicy() []block.ReferencePolicy {
	return []block.ReferencePolicy{
		{Field: "address", Constants: true},
		{Field: "method", Constants: true, Variables: true, InlineOnly: true},
		{Field: "tls", Constants: false},
		{Field: "request", Constants: true, Variables: true, InlineOnly: true},
		{Field: "metadata", Constants: true, Variables: true, InlineOnly: true},
	}
}

// Invoke dynamically calls a unary gRPC method.
func (b *Block) Invoke(ctx context.Context, config Config) (string, error) {
	if b.invoke != nil {
		return b.invoke(ctx, config)
	}
	if config.Address == "" {
		return "", fmt.Errorf("step address is required")
	}

	client := NewClient()
	defer client.Close()

	err := client.Connect(ctx, config.Address, config.TLS)
	if err != nil {
		return "", err
	}

	reflector := NewReflectionService(client)
	methodDescriptor, err := reflector.ResolveMethod(ctx, config.Method)
	if err != nil {
		return "", err
	}

	builder := NewRequestBuilder()
	requestMessage, err := builder.Build(methodDescriptor.Input(), config.Request)
	if err != nil {
		return "", err
	}

	return client.InvokeUnary(ctx, methodDescriptor, requestMessage, config.Metadata)
}
