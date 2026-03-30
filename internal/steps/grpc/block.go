// Package grpc implements gRPC workflow steps.
package grpc

import (
	"context"
	"fmt"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/steps"
)

// Block implements the workflow block contract for unary gRPC requests.
type Block struct {
	invoke func(ctx context.Context, config Config) (string, error)
}

// New creates a gRPC workflow block.
func New() *Block {
	return &Block{}
}

// Type returns the block type identifier.
func (b *Block) Type() string {
	return steps.BlockTypeGRPC
}

// SupportsResponseMapping reports whether the block exposes response mapping.
func (b *Block) SupportsResponseMapping() bool {
	return true
}

// ReferencePolicy describes which fields accept constants and runtime variables.
func (b *Block) ReferencePolicy() []block.ReferencePolicy {
	return []block.ReferencePolicy{
		{Field: "address", Constants: true},
		{Field: "method", Constants: true, Variables: true, InlineOnly: true},
		{Field: "tls", Constants: false},
		{Field: "request", Constants: true, Variables: true, InlineOnly: true},
		{Field: "metadata", Constants: true, Variables: true, InlineOnly: true},
	}
}

// Close implements block.Closer for registry lifecycle compatibility.
// Block is stateless, so there is no persistent resource to release.
func (b *Block) Close() {
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
