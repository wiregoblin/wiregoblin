package http //nolint:revive // package name matches the block domain intentionally.

import (
	"context"
	"crypto/tls"
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

const blockType = "http"

// httpClient is a shared client with a request timeout.
var httpClient = &http.Client{Timeout: 30 * time.Second}

// Block executes HTTP requests.
type Block struct{}

// New creates an HTTP workflow block.
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
		{Field: "url", Constants: true, Variables: true, InlineOnly: true},
		{Field: "body", Constants: true, Variables: true, InlineOnly: true},
		{Field: "headers", Constants: true, Variables: true, InlineOnly: true},
		{Field: "sign", Constants: true, Variables: true, InlineOnly: true},
	}
}

// Validate checks the minimal HTTP fields.
func (b *Block) Validate(step model.Step) error {
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
	step model.Step,
) (*block.Result, error) {
	config := decodeConfig(step)
	startedAt := time.Now()

	signer, err := newSigner(config.Signing)
	if err != nil {
		return nil, fmt.Errorf("auth config: %w", err)
	}

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

	if signer != nil {
		newBody, signErr := signer.Sign(req, []byte(config.Body))
		if signErr != nil {
			return nil, fmt.Errorf("sign request: %w", signErr)
		}
		if config.Signing.BodyField != "" {
			config.Body = string(newBody)
			req.Body = io.NopCloser(strings.NewReader(config.Body))
		}
	}

	// #nosec G704 -- Workflow HTTP requests intentionally target user-configured endpoints.
	resp, err := clientForConfig(httpClient, config).Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read http response: %w", err)
	}
	responseTimeMS := time.Since(startedAt).Milliseconds()

	body := string(rawBody)
	output := map[string]any{
		"status":         resp.StatusCode,
		"statusText":     resp.Status,
		"responseTimeMs": responseTimeMS,
		"body":           decodeResponseBody(body),
	}
	request := map[string]any{
		"method":  config.Method,
		"url":     config.URL,
		"headers": cloneHeaders(req.Header),
	}
	if config.Body != "" {
		request["body"] = config.Body
	}
	exports := map[string]string{
		"statusCode":     strconv.Itoa(resp.StatusCode),
		"statusText":     resp.Status,
		"responseTimeMs": strconv.FormatInt(responseTimeMS, 10),
		"body":           body,
	}

	if resp.StatusCode >= 400 {
		return &block.Result{Output: output, Exports: exports, Request: request}, fmt.Errorf("http %s", resp.Status)
	}

	return &block.Result{Output: output, Exports: exports, Request: request}, nil
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

func cloneHeaders(headers http.Header) map[string]any {
	if len(headers) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(headers))
	for key, values := range headers {
		if len(values) == 1 {
			out[key] = values[0]
			continue
		}
		items := make([]any, len(values))
		for i, value := range values {
			items[i] = value
		}
		out[key] = items
	}
	return out
}

func clientForConfig(base *http.Client, config httpConfig) *http.Client {
	client := *base
	if config.TimeoutSeconds > 0 {
		client.Timeout = time.Duration(config.TimeoutSeconds) * time.Second
	}
	if config.TLSSkipVerify {
		client.Transport = transportWithTLSSkipVerify(base.Transport)
	}
	return &client
}

func transportWithTLSSkipVerify(base http.RoundTripper) http.RoundTripper {
	switch typed := base.(type) {
	case *http.Transport:
		transport := typed.Clone()
		if transport.TLSClientConfig == nil {
			transport.TLSClientConfig = &tls.Config{
				MinVersion: tls.VersionTLS12,
			} //nolint:gosec // controlled by explicit tls_skip_verify.
		}
		transport.TLSClientConfig.InsecureSkipVerify = true //nolint:gosec // controlled by explicit tls_skip_verify.
		return transport
	default:
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: true, //nolint:gosec // controlled by explicit tls_skip_verify.
		}
		return transport
	}
}
