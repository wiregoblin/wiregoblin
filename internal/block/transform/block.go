// Package transform implements structured data transformation workflow steps.
package transform

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/model"
)

const blockType = "transform"

// Block builds structured values from an inline template and optional casts.
type Block struct{}

// New creates a transform workflow block.
func New() *Block {
	return &Block{}
}

// Type returns the transform block type identifier.
func (b *Block) Type() model.BlockType {
	return blockType
}

// ReferencePolicy describes which transform fields accept constants and runtime variables.
func (b *Block) ReferencePolicy() []block.ReferencePolicy {
	return []block.ReferencePolicy{
		{Field: "value", Constants: true, Variables: true, InlineOnly: true},
		{Field: "casts", Constants: true, Variables: true, InlineOnly: true},
	}
}

// SupportsResponseMapping reports whether transform output can be assigned into runtime variables.
func (b *Block) SupportsResponseMapping() bool {
	return true
}

// Validate checks the transform configuration.
func (b *Block) Validate(step model.Step) error {
	config := decodeConfig(step)
	if config.Value == nil {
		return fmt.Errorf("transform value is required")
	}
	for path, castType := range config.Casts {
		if strings.TrimSpace(path) == "" {
			return fmt.Errorf("transform cast path is required")
		}
		if !isSupportedCast(castType) {
			return fmt.Errorf("transform cast %q for path %q is not supported", castType, path)
		}
	}
	for name, spec := range config.Regex {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("transform regex name is required")
		}
		if strings.TrimSpace(spec.Pattern) == "" {
			return fmt.Errorf("transform regex pattern for %q is required", name)
		}
		if _, err := regexp.Compile(spec.Pattern); err != nil {
			return fmt.Errorf("transform regex pattern for %q is invalid: %w", name, err)
		}
		if spec.Group < 0 {
			return fmt.Errorf("transform regex group for %q must be non-negative", name)
		}
	}
	return nil
}

// Execute renders the transformed value, serialized JSON view, and optional regex extraction results.
func (b *Block) Execute(_ context.Context, _ *block.RunContext, step model.Step) (*block.Result, error) {
	config := decodeConfig(step)
	value, err := cloneValue(config.Value)
	if err != nil {
		return nil, err
	}
	for path, castType := range config.Casts {
		if err := applyCast(&value, path, castType); err != nil {
			return nil, err
		}
	}
	extracted, err := extractRegexMatches(value, config.Regex)
	if err != nil {
		return nil, err
	}
	rendered, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("transform marshal output: %w", err)
	}
	output := map[string]any{
		"value": value,
		"json":  string(rendered),
	}
	if len(extracted) != 0 {
		extractedAny := make(map[string]any, len(extracted))
		for key, value := range extracted {
			extractedAny[key] = value
		}
		output["extracted"] = extractedAny
	}
	return &block.Result{Output: output}, nil
}

func isSupportedCast(castType string) bool {
	switch strings.ToLower(strings.TrimSpace(castType)) {
	case "string", "int", "float", "bool", "json":
		return true
	default:
		return false
	}
}

func cloneValue(value any) (any, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("transform clone value: %w", err)
	}
	var cloned any
	if err := json.Unmarshal(body, &cloned); err != nil {
		return nil, fmt.Errorf("transform clone value: %w", err)
	}
	return cloned, nil
}

func applyCast(target *any, path, castType string) error {
	if target == nil {
		return fmt.Errorf("transform target is required")
	}
	parts := splitPath(path)
	if len(parts) == 0 {
		converted, err := castValue(*target, castType)
		if err != nil {
			return err
		}
		*target = converted
		return nil
	}
	return applyCastAtPath(target, parts, castType)
}

func applyCastAtPath(target *any, parts []string, castType string) error {
	if len(parts) == 0 {
		converted, err := castValue(*target, castType)
		if err != nil {
			return err
		}
		*target = converted
		return nil
	}

	switch current := (*target).(type) {
	case map[string]any:
		child, ok := current[parts[0]]
		if !ok {
			return fmt.Errorf("transform cast path %q not found", strings.Join(parts, "."))
		}
		if err := applyCastAtPath(&child, parts[1:], castType); err != nil {
			return err
		}
		current[parts[0]] = child
		return nil
	case []any:
		index, err := strconv.Atoi(parts[0])
		if err != nil || index < 0 || index >= len(current) {
			return fmt.Errorf("transform cast path %q not found", strings.Join(parts, "."))
		}
		child := current[index]
		if err := applyCastAtPath(&child, parts[1:], castType); err != nil {
			return err
		}
		current[index] = child
		return nil
	default:
		return fmt.Errorf("transform cast path %q not found", strings.Join(parts, "."))
	}
}

func splitPath(path string) []string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, ".")
}

func castValue(value any, castType string) (any, error) {
	switch strings.ToLower(strings.TrimSpace(castType)) {
	case "string":
		return fmt.Sprint(value), nil
	case "int":
		return strconv.Atoi(strings.TrimSpace(fmt.Sprint(value)))
	case "float":
		return strconv.ParseFloat(strings.TrimSpace(fmt.Sprint(value)), 64)
	case "bool":
		return strconv.ParseBool(strings.TrimSpace(fmt.Sprint(value)))
	case "json":
		var decoded any
		if err := json.Unmarshal([]byte(strings.TrimSpace(fmt.Sprint(value))), &decoded); err != nil {
			return nil, fmt.Errorf("transform json cast failed: %w", err)
		}
		return decoded, nil
	default:
		return nil, fmt.Errorf("unsupported transform cast %q", castType)
	}
}

func extractRegexMatches(value any, specs map[string]regexExtractConfig) (map[string]string, error) {
	if len(specs) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(specs))
	for name, spec := range specs {
		source, err := readRegexSource(value, spec.From)
		if err != nil {
			return nil, fmt.Errorf("transform regex %q: %w", name, err)
		}
		re, err := regexp.Compile(spec.Pattern)
		if err != nil {
			return nil, err
		}
		match := re.FindStringSubmatch(source)
		if len(match) == 0 {
			return nil, fmt.Errorf("transform regex %q did not match", name)
		}
		if spec.Group >= len(match) {
			return nil, fmt.Errorf("transform regex %q group %d out of range", name, spec.Group)
		}
		out[name] = match[spec.Group]
	}
	return out, nil
}

func readRegexSource(value any, from string) (string, error) {
	trimmed := strings.TrimSpace(from)
	if trimmed == "" || trimmed == "value" {
		return fmt.Sprint(value), nil
	}
	path := strings.TrimPrefix(trimmed, "value.")
	current := value
	for _, part := range splitPath(path) {
		switch typed := current.(type) {
		case map[string]any:
			next, ok := typed[part]
			if !ok {
				return "", fmt.Errorf("path %q not found", trimmed)
			}
			current = next
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(typed) {
				return "", fmt.Errorf("path %q not found", trimmed)
			}
			current = typed[index]
		default:
			return "", fmt.Errorf("path %q not found", trimmed)
		}
	}
	return fmt.Sprint(current), nil
}
