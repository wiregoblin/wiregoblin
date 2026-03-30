package assert

import "github.com/wiregoblin/wiregoblin/internal/models"

// Example YAML config:
//
// variable: Status
// operator: eq
// expected: ok
// error_message: status must be ok

type assertConfig struct {
	Variable     string
	Operator     string
	Expected     string
	ErrorMessage string
}

func decodeConfig(step models.Step) assertConfig {
	config := assertConfig{}
	if value, ok := step.Config["variable"].(string); ok {
		config.Variable = value
	}
	if value, ok := step.Config["operator"].(string); ok {
		config.Operator = value
	}
	if value, ok := step.Config["expected"].(string); ok {
		config.Expected = value
	}
	if value, ok := step.Config["error_message"].(string); ok {
		config.ErrorMessage = value
	}
	return config
}
