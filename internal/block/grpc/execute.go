package grpc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/model"
)

// Execute runs one gRPC request block.
func (b *Block) Execute(
	ctx context.Context,
	runCtx *block.RunContext,
	step model.Step,
) (*block.Result, error) {
	config := decodeConfig(step)
	requestBody, err := resolveRequest(config.Request, runCtx)
	if err != nil {
		return nil, err
	}
	config.Request = requestBody
	request := map[string]any{
		"address":  config.Address,
		"tls":      config.TLS,
		"method":   config.Method,
		"request":  config.Request,
		"metadata": config.Metadata,
	}
	startedAt := time.Now()

	response, err := b.Invoke(ctx, config)
	if err != nil {
		return &block.Result{
			Exports: map[string]string{
				"responseTimeMs": fmt.Sprintf("%d", time.Since(startedAt).Milliseconds()),
			},
			Request: request,
		}, err
	}
	responseTimeMS := time.Since(startedAt).Milliseconds()

	var output any
	if err := json.Unmarshal([]byte(response), &output); err != nil {
		return &block.Result{
			Output: map[string]any{
				"body":           response,
				"responseTimeMs": responseTimeMS,
			},
			Exports: map[string]string{
				"body":           response,
				"responseTimeMs": fmt.Sprintf("%d", responseTimeMS),
			},
			Request: request,
		}, nil
	}

	return &block.Result{
		Output: map[string]any{
			"body":           output,
			"responseTimeMs": responseTimeMS,
		},
		Exports: map[string]string{
			"responseTimeMs": fmt.Sprintf("%d", responseTimeMS),
		},
		Request: request,
	}, nil
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

var requestReferencePolicy = block.ReferencePolicy{
	Constants:  true,
	Variables:  true,
	InlineOnly: true,
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
		return block.ResolveReferences(runCtx, typed, requestReferencePolicy)
	default:
		return value
	}
}

// Validate checks that the step contains the minimal gRPC fields.
func (b *Block) Validate(step model.Step) error {
	config := decodeConfig(step)
	if config.Address == "" {
		return fmt.Errorf("step address is required")
	}
	if config.Method == "" {
		return fmt.Errorf("step method is required")
	}
	return nil
}
