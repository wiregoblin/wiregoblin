// Package redact provides secret-aware masking helpers for externally visible data.
package redact

import (
	"slices"
	"strings"
)

const placeholder = "[REDACTED]"

// SecretValues returns sorted unique non-empty secret values.
func SecretValues(secrets map[string]string) []string {
	values := make([]string, 0, len(secrets))
	for _, value := range secrets {
		if value != "" {
			values = append(values, value)
		}
	}

	slices.Sort(values)
	return slices.Compact(values)
}

// String masks all secret substrings in value.
func String(value string, secretValues []string) string {
	redacted := value
	for _, secret := range secretValues {
		if secret == "" {
			continue
		}
		redacted = strings.ReplaceAll(redacted, secret, placeholder)
	}
	return redacted
}

// Strings clones and redacts a string map.
func Strings(values map[string]string, secretValues []string) map[string]string {
	if values == nil {
		return nil
	}

	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = String(value, secretValues)
	}
	return out
}

// Value clones and redacts supported JSON-like values.
func Value(value any, secretValues []string) any {
	switch typed := value.(type) {
	case string:
		return String(typed, secretValues)
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = Value(item, secretValues)
		}
		return out
	case map[string]string:
		return Strings(typed, secretValues)
	case []any:
		out := make([]any, len(typed))
		for index, item := range typed {
			out[index] = Value(item, secretValues)
		}
		return out
	default:
		return value
	}
}
