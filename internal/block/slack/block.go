// Package slack implements Slack Incoming Webhook workflow steps.
package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/model"
)

const blockType = "slack"

var httpClient = &http.Client{Timeout: 30 * time.Second}

// Block executes Slack Incoming Webhook requests.
type Block struct{}

// New creates a Slack workflow block.
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
		{Field: "webhook_url", Constants: true},
		{Field: "text", Constants: true, Variables: true, InlineOnly: true},
		{Field: "channel", Constants: true, Variables: true, InlineOnly: true},
		{Field: "username", Constants: true, Variables: true, InlineOnly: true},
		{Field: "icon_emoji", Constants: true, Variables: true, InlineOnly: true},
		{Field: "blocks", Constants: true, Variables: true, InlineOnly: true},
	}
}

// Validate checks the minimal Slack fields.
func (b *Block) Validate(step model.Step) error {
	config := decodeConfig(step)
	if config.WebhookURL == "" {
		return fmt.Errorf("slack webhook_url is required")
	}
	if config.Text == "" && config.Blocks == "" {
		return fmt.Errorf("slack text or blocks is required")
	}
	if config.TimeoutSeconds < 0 {
		return fmt.Errorf("slack timeout_seconds must be non-negative")
	}
	if config.Blocks != "" {
		var blocks any
		if err := json.Unmarshal([]byte(config.Blocks), &blocks); err != nil {
			return fmt.Errorf("slack blocks must be valid json")
		}
	}
	return nil
}

// Execute sends one Slack Incoming Webhook request.
func (b *Block) Execute(
	ctx context.Context,
	_ *block.RunContext,
	step model.Step,
) (*block.Result, error) {
	return execute(ctx, decodeConfig(step))
}

func execute(ctx context.Context, config slackConfig) (*block.Result, error) {
	payload := map[string]any{}
	if config.Text != "" {
		payload["text"] = config.Text
	}
	if config.Channel != "" {
		payload["channel"] = config.Channel
	}
	if config.Username != "" {
		payload["username"] = config.Username
	}
	if config.IconEmoji != "" {
		payload["icon_emoji"] = config.IconEmoji
	}
	if config.Blocks != "" {
		var blocks any
		if err := json.Unmarshal([]byte(config.Blocks), &blocks); err != nil {
			return nil, fmt.Errorf("decode slack blocks: %w", err)
		}
		payload["blocks"] = blocks
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal slack request: %w", err)
	}
	request := map[string]any{
		"method":  http.MethodPost,
		"url":     config.WebhookURL,
		"headers": map[string]any{"Content-Type": "application/json"},
		"body":    payload,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, config.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build slack request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// #nosec G704 -- Slack webhook requests intentionally target configured endpoints.
	resp, err := clientForTimeout(httpClient, config.TimeoutSeconds).Do(req)
	if err != nil {
		return &block.Result{Request: request}, fmt.Errorf("slack request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read slack response: %w", err)
	}

	responseBody := string(rawBody)
	output := map[string]any{
		"status":     resp.StatusCode,
		"statusText": resp.Status,
		"body":       responseBody,
	}
	exports := map[string]string{
		"statusCode": strconv.Itoa(resp.StatusCode),
		"statusText": resp.Status,
		"body":       responseBody,
	}

	if resp.StatusCode >= 400 {
		return &block.Result{Output: output, Exports: exports, Request: request}, fmt.Errorf("slack %s", resp.Status)
	}

	return &block.Result{Output: output, Exports: exports, Request: request}, nil
}

func clientForTimeout(base *http.Client, timeoutSeconds int) *http.Client {
	if timeoutSeconds <= 0 {
		return base
	}

	client := *base
	client.Timeout = time.Duration(timeoutSeconds) * time.Second
	return &client
}
