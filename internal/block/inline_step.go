package block

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/wiregoblin/wiregoblin/internal/model"
)

// DecodeInlineStep converts one embedded block config into a runtime step.
func DecodeInlineStep(raw any, defaultName string) (model.Step, error) {
	configMap, ok := raw.(map[string]any)
	if !ok {
		return model.Step{}, fmt.Errorf("block must be an object")
	}

	step := model.Step{
		Name:    defaultName,
		Enabled: true,
		Config:  map[string]any{},
	}

	for rawKey, value := range configMap {
		key := normalizeConfigKey(rawKey)
		switch key {
		case "name":
			step.Name = strings.TrimSpace(fmt.Sprint(value))
		case "type":
			step.Type = model.BlockType(strings.ToLower(strings.TrimSpace(fmt.Sprint(value))))
		case "enabled":
			enabled, ok := toBool(value)
			if !ok {
				return model.Step{}, fmt.Errorf("block enabled must be a boolean")
			}
			step.Enabled = enabled
		case "assign":
			step.Config[key] = normalizeAssignConfig(value)
		case "target_workflow_id":
			step.Config[key] = strings.TrimSpace(fmt.Sprint(value))
		default:
			step.Config[key] = value
		}
	}

	if step.Name == "" {
		step.Name = defaultName
	}
	if step.Type == "" {
		return model.Step{}, fmt.Errorf("block type is required")
	}

	return step, nil
}

func normalizeAssignConfig(raw any) []any {
	switch typed := raw.(type) {
	case []any:
		return typed
	case map[string]any:
		entries := make([]any, 0, len(typed))
		for key, value := range typed {
			entries = append(entries, map[string]any{
				"key":  strings.TrimSpace(key),
				"path": strings.TrimSpace(fmt.Sprint(value)),
			})
		}
		return entries
	default:
		return nil
	}
}

func normalizeConfigKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}

func toBool(raw any) (bool, bool) {
	switch typed := raw.(type) {
	case bool:
		return typed, true
	case string:
		value, err := strconv.ParseBool(strings.TrimSpace(typed))
		if err != nil {
			return false, false
		}
		return value, true
	default:
		return false, false
	}
}
