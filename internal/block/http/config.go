package http //nolint:revive // package name matches the block domain intentionally.

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/wiregoblin/wiregoblin/internal/model"
)

// Example YAML config:
//
// method: POST
// url: https://api.example.com/v1/items
// body: '{"name":"demo"}'
// headers:
//   Authorization: Bearer $ApiToken

type httpConfig struct {
	Method         string
	URL            string
	Body           string
	Headers        map[string]string
	TimeoutSeconds int
	TLSSkipVerify  bool
	Signing        authConfig
}

// authConfig holds request signing configuration.
// Exactly one of Header, QueryParam, or BodyField must be set.
type authConfig struct {
	Type       string   // "hmac_sha256"
	Key        string   // signing key
	Include    []string // parts to sign: "method", "url", "body"
	Separator  string   // separator between signed parts, default "\n"
	BodyFormat string   // "raw" (default) | "sorted_json"
	Header     string   // put signature in this header
	Prefix     string   // optional prefix before the hex value
	QueryParam string   // put signature in this query param
	BodyField  string   // inject signature into JSON body at this key
}

func decodeConfig(step model.Step) httpConfig {
	if step.Config == nil {
		step.Config = map[string]any{}
	}
	config := httpConfig{}
	if v, ok := step.Config["method"].(string); ok {
		config.Method = v
	}
	if v, ok := step.Config["url"].(string); ok {
		config.URL = v
	}
	if v, ok := step.Config["body"].(string); ok {
		config.Body = v
	}
	if v, ok := step.Config["headers"].(map[string]string); ok {
		config.Headers = v
	} else if v, ok := step.Config["headers"].(map[string]any); ok {
		config.Headers = make(map[string]string, len(v))
		for k, val := range v {
			config.Headers[k] = fmt.Sprint(val)
		}
	}
	config.TimeoutSeconds = decodeTimeoutSeconds(step.Config["timeout_seconds"])
	if v, ok := step.Config["tls_skip_verify"].(bool); ok {
		config.TLSSkipVerify = v
	}
	config.Signing = decodeAuth(step.Config["sign"])
	return config
}

func decodeAuth(raw any) authConfig {
	m, ok := raw.(map[string]any)
	if !ok {
		return authConfig{}
	}
	cfg := authConfig{}
	if v, ok := m["type"].(string); ok {
		cfg.Type = v
	}
	if v, ok := m["key"].(string); ok {
		cfg.Key = v
	}
	if v, ok := m["separator"].(string); ok {
		cfg.Separator = v
	}
	if v, ok := m["body_format"].(string); ok {
		cfg.BodyFormat = v
	}
	if v, ok := m["header"].(string); ok {
		cfg.Header = v
	}
	if v, ok := m["prefix"].(string); ok {
		cfg.Prefix = v
	}
	if v, ok := m["query_param"].(string); ok {
		cfg.QueryParam = v
	}
	if v, ok := m["body_field"].(string); ok {
		cfg.BodyField = v
	}
	switch raw := m["include"].(type) {
	case []string:
		cfg.Include = raw
	case []any:
		for _, item := range raw {
			if s, ok := item.(string); ok {
				cfg.Include = append(cfg.Include, s)
			}
		}
	}
	return cfg
}

func decodeTimeoutSeconds(raw any) int {
	switch typed := raw.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		value, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return value
		}
	}
	return 0
}
