// Package openai implements OpenAI-compatible workflow steps.
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/model"
)

const blockType = "openai"

var httpClient = &http.Client{Timeout: 30 * time.Second}

// Block executes OpenAI-compatible chat completion requests.
type Block struct{}

// New creates an OpenAI-compatible workflow block.
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
		{Field: "base_url", Constants: true},
		{Field: "token", Constants: true},
		{Field: "system_prompt", Constants: true, Variables: true, InlineOnly: true},
		{Field: "user_prompt", Constants: true, Variables: true, InlineOnly: true},
		{Field: "headers", Constants: true, Variables: true, InlineOnly: true},
	}
}

// Validate checks the minimal fields.
func (b *Block) Validate(step model.Step) error {
	config := decodeConfig(step)
	if config.BaseURL == "" {
		return fmt.Errorf("openai base url is required")
	}
	if config.Model == "" {
		return fmt.Errorf("openai model is required")
	}
	if config.UserPrompt == "" {
		return fmt.Errorf("openai user prompt is required")
	}
	if config.TimeoutSeconds < 0 {
		return fmt.Errorf("openai timeout_seconds must be non-negative")
	}
	return nil
}

// Execute runs one OpenAI-compatible chat completion request.
func (b *Block) Execute(
	ctx context.Context,
	_ *block.RunContext,
	step model.Step,
) (*block.Result, error) {
	return execute(ctx, decodeConfig(step))
}

func execute(ctx context.Context, config openaiConfig) (*block.Result, error) {
	endpoint := buildEndpoint(config.BaseURL)

	payload := map[string]any{
		"model":    config.Model,
		"messages": buildMessages(config.SystemPrompt, config.UserPrompt),
	}
	if config.Temperature != "" {
		value, err := strconv.ParseFloat(config.Temperature, 64)
		if err != nil {
			return nil, fmt.Errorf("openai temperature must be a number")
		}
		payload["temperature"] = value
	}
	if config.MaxTokens != "" {
		value, err := strconv.Atoi(config.MaxTokens)
		if err != nil {
			return nil, fmt.Errorf("openai max tokens must be an integer")
		}
		payload["max_tokens"] = value
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal openai request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build openai request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for key, value := range config.Headers {
		req.Header.Set(key, value)
	}
	if strings.TrimSpace(config.Token) != "" && req.Header.Get("Authorization") == "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(config.Token))
	}

	// #nosec G704 -- OpenAI-compatible requests intentionally target configured endpoints.
	resp, err := clientForTimeout(httpClient, config.TimeoutSeconds).Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read openai response: %w", err)
	}

	var parsed any
	if err := json.Unmarshal(rawBody, &parsed); err != nil {
		return nil, fmt.Errorf("openai response body must be valid json: %w", err)
	}

	output := map[string]any{
		"status":     resp.StatusCode,
		"statusText": resp.Status,
		"body":       parsed,
	}
	exports := map[string]string{
		"statusCode": strconv.Itoa(resp.StatusCode),
		"statusText": resp.Status,
		"body":       string(rawBody),
	}

	if resp.StatusCode >= 400 {
		return &block.Result{Output: output, Exports: exports}, fmt.Errorf("openai %s", resp.Status)
	}

	return &block.Result{Output: output, Exports: exports}, nil
}

func buildMessages(systemPrompt, userPrompt string) []map[string]string {
	messages := make([]map[string]string, 0, 2)
	if strings.TrimSpace(systemPrompt) != "" {
		messages = append(messages, map[string]string{
			"role":    "system",
			"content": systemPrompt,
		})
	}
	messages = append(messages, map[string]string{
		"role":    "user",
		"content": userPrompt,
	})
	return messages
}

func buildEndpoint(baseURL string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	switch {
	case strings.HasSuffix(trimmed, "/chat/completions"):
		return trimmed
	case strings.HasSuffix(trimmed, "/v1"):
		return trimmed + "/chat/completions"
	default:
		return trimmed + "/v1/chat/completions"
	}
}

func clientForTimeout(base *http.Client, timeoutSeconds int) *http.Client {
	if timeoutSeconds <= 0 {
		return base
	}

	client := *base
	client.Timeout = time.Duration(timeoutSeconds) * time.Second
	return &client
}
