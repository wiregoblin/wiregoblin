package model

import "fmt"

// ValidateWorkflow checks the minimal workflow invariants before execution.
func ValidateWorkflow(definition *Workflow) error {
	if definition == nil {
		return fmt.Errorf("workflow is required")
	}
	if definition.Name == "" {
		return fmt.Errorf("workflow name is required")
	}
	if definition.TimeoutSeconds < 0 {
		return fmt.Errorf("workflow timeout_seconds must be non-negative")
	}
	for key, value := range definition.Outputs {
		if key == "" {
			return fmt.Errorf("workflow output name is required")
		}
		if value == "" {
			return fmt.Errorf("workflow output %q value is required", key)
		}
	}

	for index := range definition.Steps {
		if definition.Steps[index].Type == "" {
			return fmt.Errorf("step %d type is required", index+1)
		}
		if err := validateCondition(definition.Steps[index].Condition); err != nil {
			return fmt.Errorf("step %d condition: %w", index+1, err)
		}
	}
	for index := range definition.OnErrorSteps {
		if definition.OnErrorSteps[index].Type == "" {
			return fmt.Errorf("on error step %d type is required", index+1)
		}
		if err := validateCondition(definition.OnErrorSteps[index].Condition); err != nil {
			return fmt.Errorf("on error step %d condition: %w", index+1, err)
		}
	}

	return nil
}

func validateCondition(condition *Condition) error {
	if condition == nil {
		return nil
	}
	if condition.Variable == "" {
		return fmt.Errorf("variable is required")
	}
	if condition.Operator == "" {
		return fmt.Errorf("operator is required")
	}
	return nil
}
