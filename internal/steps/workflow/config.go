package workflowblock

import (
	"fmt"
	"strings"

	"github.com/wiregoblin/wiregoblin/internal/models"
)

// Example YAML config:
//
// target_workflow_uid: child-workflow-id
// input_mapping:
//   RequestID: "@RequestID"

type workflowConfig struct {
	TargetWorkflowUID string
	InputMapping      map[string]string
}

func decodeConfig(step models.Step) workflowConfig {
	config := workflowConfig{}
	if value, ok := step.Config["target_workflow_uid"].(string); ok {
		config.TargetWorkflowUID = strings.TrimSpace(value)
	}
	switch value := step.Config["input_mapping"].(type) {
	case map[string]string:
		config.InputMapping = make(map[string]string, len(value))
		for key, item := range value {
			config.InputMapping[key] = strings.TrimSpace(item)
		}
	case map[string]any:
		config.InputMapping = make(map[string]string, len(value))
		for key, item := range value {
			config.InputMapping[key] = strings.TrimSpace(fmt.Sprint(item))
		}
	}
	return config
}
