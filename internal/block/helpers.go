package block

import (
	"fmt"
	"strings"
)

// CloneStringMap returns a shallow copy of src.
func CloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return map[string]string{}
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// CloneStepResults returns a shallow copy of a step-results map.
func CloneStepResults(src map[string]any) map[string]any {
	if len(src) == 0 {
		return map[string]any{}
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// CloneRunContext returns a shallow clone of src with all string maps deep-copied.
func CloneRunContext(src *RunContext) *RunContext {
	if src == nil {
		return nil
	}
	return &RunContext{
		ProjectID:           src.ProjectID,
		Constants:           CloneStringMap(src.Constants),
		Secrets:             CloneStringMap(src.Secrets),
		SecretVariables:     CloneStringMap(src.SecretVariables),
		Variables:           CloneStringMap(src.Variables),
		Builtins:            CloneStringMap(src.Builtins),
		StepResults:         CloneStepResults(src.StepResults),
		ExecuteWorkflow:     src.ExecuteWorkflow,
		ExecuteStep:         src.ExecuteStep,
		ExecuteIsolatedStep: src.ExecuteIsolatedStep,
	}
}

// HasNestedAssignments reports whether a block config map contains non-empty assign entries.
func HasNestedAssignments(config map[string]any) bool {
	if len(config) == 0 {
		return false
	}
	assign, ok := config["assign"]
	if !ok || assign == nil {
		return false
	}
	switch typed := assign.(type) {
	case map[string]any:
		return len(typed) != 0
	case []any:
		return len(typed) != 0
	default:
		return true
	}
}

// CollectedName strips the leading $ sigil from a collect target name.
func CollectedName(target string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(target), "$"))
}

// ReadCollectedValue reads a dot-delimited path from a map, matching keys case-insensitively.
func ReadCollectedValue(results map[string]any, path string) (any, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, false
	}
	parts := strings.Split(path, ".")
	var current any = results
	for _, part := range parts {
		if part == "" {
			continue
		}
		value, ok := resolveCollectedPart(current, part)
		if !ok {
			return nil, false
		}
		current = value
	}
	return current, true
}

// DecodeCollect parses a collect config value into a $target→path map.
func DecodeCollect(raw any) map[string]string {
	result := map[string]string{}
	switch typed := raw.(type) {
	case map[string]string:
		for key, value := range typed {
			key = strings.TrimSpace(key)
			value = strings.TrimSpace(value)
			if key != "" && value != "" {
				result[key] = value
			}
		}
	case map[string]any:
		for key, value := range typed {
			key = strings.TrimSpace(key)
			path := strings.TrimSpace(fmt.Sprint(value))
			if key != "" && path != "" {
				result[key] = path
			}
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func resolveCollectedPart(current any, key string) (any, bool) {
	switch typed := current.(type) {
	case map[string]any:
		return resolveCollectedKey(typed, key)
	case map[string]string:
		return resolveCollectedStringKey(typed, key)
	default:
		return nil, false
	}
}

func resolveCollectedKey(m map[string]any, key string) (any, bool) {
	if value, ok := m[key]; ok {
		return value, true
	}
	lowerKey := strings.ToLower(key)
	for k, value := range m {
		if strings.ToLower(k) == lowerKey {
			return value, true
		}
	}
	return nil, false
}

func resolveCollectedStringKey(m map[string]string, key string) (any, bool) {
	if value, ok := m[key]; ok {
		return value, true
	}
	lowerKey := strings.ToLower(key)
	for k, value := range m {
		if strings.ToLower(k) == lowerKey {
			return value, true
		}
	}
	return nil, false
}
