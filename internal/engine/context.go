// Package engine contains the workflow execution engine and runtime orchestration.
package engine

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/model"
)

// NewRunContext builds a workflow-scoped execution context from project and workflow definitions.
// Secrets stay in the @ namespace, mutable runtime values resolve through $, and read-only runtime built-ins use !.
func NewRunContext(project *model.Project, definition *model.Workflow) *block.RunContext {
	runCtx := &block.RunContext{
		Constants:       map[string]string{},
		Secrets:         map[string]string{},
		SecretVariables: map[string]string{},
		Variables:       map[string]string{},
		Builtins: func() map[string]string {
			now := time.Now().UTC()
			return map[string]string{
				"RunID":     uuid.New().String(),
				"StartTime": now.Format(time.RFC3339),
				"StartUnix": fmt.Sprintf("%d", now.Unix()),
				"StartDate": now.Format("2006-01-02"),
			}
		}(),
		StepResults: map[string]any{},
	}

	if project != nil {
		runCtx.ProjectID = project.ID
		if project.ID != "" {
			runCtx.Builtins["ProjectID"] = project.ID
		}
		for _, entry := range project.Constants {
			if entry.Key != "" {
				runCtx.Constants[entry.Key] = entry.Value
			}
		}
		for _, entry := range project.Variables {
			if entry.Key != "" {
				runCtx.Variables[entry.Key] = entry.Value
			}
		}
		for _, entry := range project.Secrets {
			if entry.Key != "" {
				runCtx.Secrets[entry.Key] = entry.Value
			}
		}
		for _, entry := range project.SecretVariables {
			if entry.Key != "" {
				runCtx.SecretVariables[entry.Key] = entry.Value
			}
		}
	}
	if definition != nil {
		if definition.ID != "" {
			runCtx.Builtins["WorkflowID"] = definition.ID
		}
		if definition.Name != "" {
			runCtx.Builtins["WorkflowName"] = definition.Name
		}
		for _, entry := range definition.Constants {
			if entry.Key != "" {
				runCtx.Constants[entry.Key] = entry.Value
			}
		}
		for _, entry := range definition.Secrets {
			if entry.Key != "" {
				runCtx.Secrets[entry.Key] = entry.Value
			}
		}
		for _, entry := range definition.Variables {
			if entry.Key != "" {
				runCtx.Variables[entry.Key] = entry.Value
			}
		}
		for _, entry := range definition.SecretVariables {
			if entry.Key != "" {
				runCtx.SecretVariables[entry.Key] = entry.Value
			}
		}
	}

	return runCtx
}
