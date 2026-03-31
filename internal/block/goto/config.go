package gotoblock

import (
	"log"
	"strconv"
	"strings"

	"github.com/wiregoblin/wiregoblin/internal/model"
)

// Example YAML config:
//
// variable: RetryCount
// operator: lt
// expected: "3"
// target_step_id: step-id-or-key
// wait_seconds: 5

type gotoConfig struct {
	Variable     string
	Operator     string
	Expected     string
	TargetStepID string
	WaitSeconds  int
}

func decodeConfig(step model.Step) gotoConfig {
	config := gotoConfig{}
	if value, ok := step.Config["variable"].(string); ok {
		config.Variable = value
	}
	if value, ok := step.Config["operator"].(string); ok {
		config.Operator = value
	}
	if value, ok := step.Config["expected"].(string); ok {
		config.Expected = value
	}
	if value, ok := step.Config["target_step_id"].(string); ok {
		config.TargetStepID = value
	}
	switch value := step.Config["wait_seconds"].(type) {
	case float64:
		config.WaitSeconds = int(value)
	case int:
		config.WaitSeconds = value
	case string:
		seconds, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			log.Printf("goto: invalid wait_seconds value %q: %v", value, err)
		} else {
			config.WaitSeconds = seconds
		}
	}
	return config
}
