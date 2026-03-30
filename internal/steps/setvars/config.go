package setvars

import (
	"fmt"

	"github.com/wiregoblin/wiregoblin/internal/models"
)

// Example YAML config:
//
// assignments:
//   - variable: Token
//     value: "@SessionToken"
//   - variable: BaseURL
//     value: $ApiURL

type setvarsConfig struct {
	Assignments []assignment
}

type assignment struct {
	Variable string
	Value    string
}

func decodeConfig(step models.Step) setvarsConfig {
	config := setvarsConfig{
		Assignments: []assignment{},
	}
	switch raw := step.Config["assignments"].(type) {
	case []map[string]any:
		for _, entry := range raw {
			config.Assignments = append(config.Assignments, assignment{
				Variable: fmt.Sprint(entry["variable"]),
				Value:    fmt.Sprint(entry["value"]),
			})
		}
	case []any:
		for _, rawEntry := range raw {
			if entry, ok := rawEntry.(map[string]any); ok {
				config.Assignments = append(config.Assignments, assignment{
					Variable: fmt.Sprint(entry["variable"]),
					Value:    fmt.Sprint(entry["value"]),
				})
			}
		}
	}
	return config
}
