package http //nolint:revive // package name matches the block domain intentionally.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Signer mutates an HTTP request by adding a signature.
// body is the raw request body bytes (before any mutation).
type Signer interface {
	Sign(req *http.Request, body []byte) (newBody []byte, err error)
}

// newSigner returns a Signer for the given auth config, or nil if auth is not configured.
func newSigner(cfg authConfig) (Signer, error) {
	if cfg.Type == "" {
		return nil, nil
	}
	destinations := 0
	if cfg.Header != "" {
		destinations++
	}
	if cfg.QueryParam != "" {
		destinations++
	}
	if cfg.BodyField != "" {
		destinations++
	}
	if destinations != 1 {
		return nil, fmt.Errorf("auth: exactly one of header, query_param, body_field must be set")
	}
	switch cfg.Type {
	case "hmac_sha256", "hmac_sha512":
		return newHMACSigner(cfg)
	case "rsa_sha256":
		return newRSASigner(cfg)
	default:
		return nil, fmt.Errorf("auth: unknown type %q", cfg.Type)
	}
}

// buildMessage assembles the string to be signed from the configured include parts.
func buildMessage(req *http.Request, body []byte, cfg authConfig) string {
	parts := cfg.Include
	if len(parts) == 0 {
		parts = []string{"body"}
	}
	sep := cfg.Separator
	if sep == "" {
		sep = "\n"
	}
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		switch part {
		case "method":
			segments = append(segments, req.Method)
		case "url":
			segments = append(segments, req.URL.String())
		case "body":
			segments = append(segments, string(body))
		}
	}
	return strings.Join(segments, sep)
}

// applyDestination writes value to the configured destination (header, query param, or body field).
// It returns the (possibly modified) body bytes.
func applyDestination(req *http.Request, body []byte, cfg authConfig, value string) ([]byte, error) {
	switch {
	case cfg.Header != "":
		req.Header.Set(cfg.Header, value)
		return body, nil
	case cfg.QueryParam != "":
		q := req.URL.Query()
		q.Set(cfg.QueryParam, value)
		req.URL.RawQuery = q.Encode()
		return body, nil
	case cfg.BodyField != "":
		newBody, err := injectBodyField(body, cfg.BodyField, value)
		if err != nil {
			return nil, fmt.Errorf("inject body field: %w", err)
		}
		req.ContentLength = int64(len(newBody))
		return newBody, nil
	}
	return body, nil
}

// prepareBody applies body_format transformations before signing.
func prepareBody(body []byte, cfg authConfig) ([]byte, error) {
	if cfg.BodyFormat == "sorted_json" && len(body) > 0 {
		return canonicalJSON(body)
	}
	return body, nil
}

// canonicalJSON sorts all object keys recursively and serializes to compact JSON.
func canonicalJSON(data []byte) ([]byte, error) {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return json.Marshal(sortedValue(v))
}

// sortedValue recursively converts maps to sorted representations.
// json.Marshal serializes map[string]any with sorted keys by default in Go.
func sortedValue(v any) any {
	switch typed := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for k, val := range typed {
			out[k] = sortedValue(val)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = sortedValue(item)
		}
		return out
	default:
		return v
	}
}

// injectBodyField adds a top-level field to a JSON object body.
func injectBodyField(body []byte, field, value string) ([]byte, error) {
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return nil, fmt.Errorf("body_field requires a JSON object body: %w", err)
	}
	obj[field] = value
	return json.Marshal(obj)
}
