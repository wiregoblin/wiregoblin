package setvars

import (
	"fmt"

	"github.com/wiregoblin/wiregoblin/internal/model"
)

// Example YAML config:
//
// set:
//   $Token: "$SessionToken"
//   $BaseURL: @ApiURL

type setvarsConfig struct {
	Set map[string]string
}

func decodeConfig(step model.Step) setvarsConfig {
	config := setvarsConfig{
		Set: map[string]string{},
	}
	switch raw := step.Config["set"].(type) {
	case map[string]string:
		for key, value := range raw {
			config.Set[key] = value
		}
	case map[string]any:
		for key, value := range raw {
			config.Set[key] = fmt.Sprint(value)
		}
	}
	return config
}
