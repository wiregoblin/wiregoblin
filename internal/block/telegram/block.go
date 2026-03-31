// Package telegram implements Telegram notification workflow steps.
package telegram

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

const blockType = "telegram"

var httpClient = &http.Client{Timeout: 30 * time.Second}

const defaultBaseURL = "https://api.telegram.org"

// Block sends Telegram notifications.
type Block struct{}

// New creates a Telegram workflow block.
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
		{Field: "chat_id", Constants: true, Variables: true, InlineOnly: true},
		{Field: "message", Constants: true, Variables: true, InlineOnly: true},
		{Field: "parse_mode", Constants: true, Variables: true, InlineOnly: true},
	}
}

// Validate checks the minimal Telegram fields.
func (b *Block) Validate(step model.Step) error {
	config := decodeConfig(step)
	if config.Token == "" {
		return fmt.Errorf("telegram bot token is required")
	}
	if config.ChatID == "" {
		return fmt.Errorf("telegram chat id is required")
	}
	if config.Message == "" {
		return fmt.Errorf("telegram message is required")
	}
	if config.TimeoutSeconds < 0 {
		return fmt.Errorf("telegram timeout_seconds must be non-negative")
	}
	return nil
}

// Execute sends one Telegram message.
func (b *Block) Execute(
	ctx context.Context,
	_ *block.RunContext,
	step model.Step,
) (*block.Result, error) {
	return execute(ctx, decodeConfig(step))
}

func execute(ctx context.Context, config telegramConfig) (*block.Result, error) {
	payload := map[string]any{
		"chat_id": config.ChatID,
		"text":    config.Message,
	}
	if strings.TrimSpace(config.ParseMode) != "" {
		payload["parse_mode"] = config.ParseMode
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal telegram request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		buildEndpoint(config.BaseURL, config.Token),
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("build telegram request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// #nosec G704 -- Telegram requests intentionally target configured API endpoints.
	resp, err := clientForTimeout(httpClient, config.TimeoutSeconds).Do(req)
	if err != nil {
		return nil, fmt.Errorf("telegram request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read telegram response: %w", err)
	}

	var parsed any
	if err := json.Unmarshal(rawBody, &parsed); err != nil {
		return nil, fmt.Errorf("telegram response body must be valid json: %w", err)
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
		return &block.Result{Output: output, Exports: exports}, fmt.Errorf("telegram %s", resp.Status)
	}

	return &block.Result{Output: output, Exports: exports}, nil
}

func buildEndpoint(baseURL, token string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		trimmed = defaultBaseURL
	}
	return fmt.Sprintf("%s/bot%s/sendMessage", trimmed, strings.TrimSpace(token))
}

func clientForTimeout(base *http.Client, timeoutSeconds int) *http.Client {
	if timeoutSeconds <= 0 {
		return base
	}

	client := *base
	client.Timeout = time.Duration(timeoutSeconds) * time.Second
	return &client
}
