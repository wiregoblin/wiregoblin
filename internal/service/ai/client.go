// Package ai provides project-configured AI client helpers and prompt services.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

func doJSONRequest(ctx context.Context, timeoutSeconds int, method, url string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal ai request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build ai request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := clientForTimeout(httpClient, timeoutSeconds)
	// #nosec G107,G704 -- URL comes from explicit project AI provider configuration.
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("ai request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read ai response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("ai %s: %s", resp.Status, strings.TrimSpace(string(rawBody)))
	}
	if err := json.Unmarshal(rawBody, out); err != nil {
		return fmt.Errorf("decode ai response: %w", err)
	}
	return nil
}

func clientForTimeout(base *http.Client, timeoutSeconds int) *http.Client {
	if timeoutSeconds <= 0 {
		return base
	}

	client := *base
	client.Timeout = time.Duration(timeoutSeconds) * time.Second
	return &client
}
