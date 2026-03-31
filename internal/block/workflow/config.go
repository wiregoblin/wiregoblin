package workflowblock

import (
	"fmt"
	"strings"

	"github.com/wiregoblin/wiregoblin/internal/model"
)

// Example YAML config:
//
// target_workflow_id: child-workflow-id
// inputs:
//   RequestID: "$RequestID"

type workflowConfig struct {
	TargetWorkflowID string
	Inputs           map[string]string
}

func decodeConfig(step model.Step) workflowConfig {
	config := workflowConfig{}
	if value, ok := step.Config["target_workflow_id"].(string); ok {
		config.TargetWorkflowID = strings.TrimSpace(value)
	}
	switch value := step.Config["inputs"].(type) {
	case map[string]string:
		config.Inputs = make(map[string]string, len(value))
		for key, item := range value {
			config.Inputs[key] = strings.TrimSpace(item)
		}
	case map[string]any:
		config.Inputs = make(map[string]string, len(value))
		for key, item := range value {
			config.Inputs[key] = strings.TrimSpace(fmt.Sprint(item))
		}
	}
	return config
}
