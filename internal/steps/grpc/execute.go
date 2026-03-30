package grpc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/models"
)

// Execute runs one gRPC request block.
func (b *Block) Execute(
	ctx context.Context,
	runCtx *block.RunContext,
	step models.Step,
) (*block.Result, error) {
	config := decodeConfig(step)
	request, err := resolveRequest(config.Request, runCtx)
	if err != nil {
		return nil, err
	}
	config.Request = request

	response, err := b.Invoke(ctx, config)
	if err != nil {
		return nil, err
	}

	var output any
	if err := json.Unmarshal([]byte(response), &output); err != nil {
		return &block.Result{Output: response}, nil
	}

	return &block.Result{Output: output}, nil
}

func resolveRequest(request string, runCtx *block.RunContext) (string, error) {
	request = strings.TrimSpace(request)
	if request == "" || runCtx == nil {
		return request, nil
	}

	var payload any
	if err := json.Unmarshal([]byte(request), &payload); err != nil {
		return "", fmt.Errorf("decode grpc request: %w", err)
	}

	resolved := resolveRequestValue(payload, runCtx)
	body, err := json.Marshal(resolved)
	if err != nil {
		return "", fmt.Errorf("encode grpc request: %w", err)
	}

	return string(body), nil
}

func resolveRequestValue(value any, runCtx *block.RunContext) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = resolveRequestValue(item, runCtx)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = resolveRequestValue(item, runCtx)
		}
		return out
	case string:
		if strings.HasPrefix(typed, "@") {
			if value, ok := runCtx.Variables[strings.TrimPrefix(typed, "@")]; ok {
				return value
			}
		}
		if strings.HasPrefix(typed, "$") {
			if value, ok := runCtx.Constants[strings.TrimPrefix(typed, "$")]; ok {
				return value
			}
		}
		return typed
	default:
		return value
	}
}

// Validate checks that the step contains the minimal gRPC fields.
func (b *Block) Validate(step models.Step) error {
	config := decodeConfig(step)
	if config.Address == "" {
		return fmt.Errorf("step address is required")
	}
	if config.Method == "" {
		return fmt.Errorf("step method is required")
	}
	return nil
}
