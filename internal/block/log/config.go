package logblock

import (
	"fmt"
	"strings"

	"github.com/wiregoblin/wiregoblin/internal/model"
)

// Example YAML config:
//
// message: "Current user is $user_id"
// level: info

type logConfig struct {
	Message string
	Level   string
}

func decodeConfig(step model.Step) logConfig {
	config := logConfig{}
	if value, ok := step.Config["message"].(string); ok {
		config.Message = strings.TrimSpace(value)
	}
	if value, ok := step.Config["level"].(string); ok {
		config.Level = strings.ToLower(strings.TrimSpace(value))
	}
	return config
}

func normalizeLevel(value string) (string, error) {
	switch value {
	case "", "info":
		return "info", nil
	case "debug", "warn", "error":
		return value, nil
	default:
		return "", fmt.Errorf("log level must be one of debug, info, warn, error")
	}
}
