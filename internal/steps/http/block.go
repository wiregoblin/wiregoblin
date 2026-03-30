package http //nolint:revive // package name matches the block domain intentionally.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/models"
	"github.com/wiregoblin/wiregoblin/internal/steps"
)

// httpClient is a shared client with a request timeout.
var httpClient = &http.Client{Timeout: 30 * time.Second}

// Block executes HTTP requests.
type Block struct{}

// New creates an HTTP workflow block.
func New() *Block {
	return &Block{}
}

// Type returns the block type identifier.
func (b *Block) Type() string {
	return steps.BlockTypeHTTP
}

// SupportsResponseMapping reports whether the block exposes response mapping.
func (b *Block) SupportsResponseMapping() bool {
	return true
}

// ReferencePolicy describes which fields accept constants and runtime variables.
func (b *Block) ReferencePolicy() []block.ReferencePolicy {
	return []block.ReferencePolicy{
		{Field: "url", Constants: true, Variables: true, InlineOnly: true},
		{Field: "body", Constants: true, Variables: true, InlineOnly: true},
		{Field: "headers", Constants: true, Variables: true, InlineOnly: true},
	}
}

// Validate checks the minimal HTTP fields.
func (b *Block) Validate(step models.Step) error {
	config := decodeConfig(step)
	if config.URL == "" {
		return fmt.Errorf("step url is required")
	}
	if config.Method == "" {
		return fmt.Errorf("step method is required")
	}
	if config.Body != "" && !isValidJSON(config.Body) {
		return fmt.Errorf("http body must be valid json")
	}
	if config.TimeoutSeconds < 0 {
		return fmt.Errorf("http timeout_seconds must be non-negative")
	}
	return nil
}

// Execute sends an HTTP request and returns the response body and status.
func (b *Block) Execute(
	ctx context.Context,
	_ *block.RunContext,
	step models.Step,
) (*block.Result, error) {
	config := decodeConfig(step)

	req, err := http.NewRequestWithContext(ctx, config.Method, config.URL, bodyReader(config.Body))
	if err != nil {
		return nil, fmt.Errorf("build http request: %w", err)
	}

	for key, value := range config.Headers {
		req.Header.Set(key, value)
	}
	if config.Body != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	// #nosec G704 -- Workflow HTTP requests intentionally target user-configured endpoints.
	resp, err := clientForTimeout(httpClient, config.TimeoutSeconds).Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read http response: %w", err)
	}

	body := string(rawBody)
	output := map[string]any{
		"status":     resp.StatusCode,
		"statusText": resp.Status,
		"body":       decodeResponseBody(body),
	}
	exports := map[string]string{
		"statusCode": strconv.Itoa(resp.StatusCode),
		"statusText": resp.Status,
		"body":       body,
	}

	if resp.StatusCode >= 400 {
		return &block.Result{Output: output, Exports: exports}, fmt.Errorf("http %s", resp.Status)
	}

	return &block.Result{Output: output, Exports: exports}, nil
}

func bodyReader(body string) io.Reader {
	if body == "" {
		return http.NoBody
	}
	return strings.NewReader(body)
}

func isValidJSON(value string) bool {
	var decoded any
	return json.Unmarshal([]byte(value), &decoded) == nil
}

func decodeResponseBody(body string) any {
	if strings.TrimSpace(body) == "" {
		return nil
	}

	var decoded any
	if err := json.Unmarshal([]byte(body), &decoded); err == nil {
		return decoded
	}
	return body
}

func clientForTimeout(base *http.Client, timeoutSeconds int) *http.Client {
	if timeoutSeconds <= 0 {
		return base
	}

	client := *base
	client.Timeout = time.Duration(timeoutSeconds) * time.Second
	return &client
}
