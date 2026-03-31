package transform

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/wiregoblin/wiregoblin/internal/model"
)

type transformConfig struct {
	Value any
	Casts map[string]string
	Regex map[string]regexExtractConfig
}

type regexExtractConfig struct {
	From    string
	Pattern string
	Group   int
}

func decodeConfig(step model.Step) transformConfig {
	config := transformConfig{
		Value: step.Config["value"],
		Casts: map[string]string{},
		Regex: map[string]regexExtractConfig{},
	}

	switch typed := step.Config["casts"].(type) {
	case map[string]string:
		for key, value := range typed {
			config.Casts[key] = value
		}
	case map[string]any:
		for key, value := range typed {
			config.Casts[key] = strings.TrimSpace(fmt.Sprint(value))
		}
	}
	if typed, ok := step.Config["regex"].(map[string]any); ok {
		for key, value := range typed {
			m, ok := value.(map[string]any)
			if !ok {
				continue
			}
			spec := regexExtractConfig{}
			if from, ok := m["from"].(string); ok {
				spec.From = strings.TrimSpace(from)
			}
			if pattern, ok := m["pattern"].(string); ok {
				spec.Pattern = pattern
			}
			switch group := m["group"].(type) {
			case int:
				spec.Group = group
			case float64:
				spec.Group = int(group)
			case string:
				if parsed, err := strconv.Atoi(strings.TrimSpace(group)); err == nil {
					spec.Group = parsed
				}
			}
			config.Regex[strings.TrimSpace(key)] = spec
		}
	}
	if len(config.Regex) == 0 {
		config.Regex = nil
	}

	return config
}
