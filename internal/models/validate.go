package models

import "fmt"

// ValidateWorkflow checks the minimal workflow invariants before execution.
func ValidateWorkflow(definition *Workflow) error {
	if definition == nil {
		return fmt.Errorf("workflow is required")
	}
	if definition.Name == "" {
		return fmt.Errorf("workflow name is required")
	}

	for index := range definition.Steps {
		if definition.Steps[index].Type == "" {
			return fmt.Errorf("step %d type is required", index+1)
		}
	}
	for index := range definition.OnErrorSteps {
		if definition.OnErrorSteps[index].Type == "" {
			return fmt.Errorf("on error step %d type is required", index+1)
		}
	}

	return nil
}
