// Package engine contains the workflow execution engine and runtime orchestration.
package engine

import (
	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/models"
)

// NewRunContext builds a workflow-scoped execution context from project and workflow definitions.
// Secrets are pre-resolved (from environment variables) and stored in Variables.
func NewRunContext(project *models.Project, definition *models.Workflow) *block.RunContext {
	runCtx := &block.RunContext{
		Constants:   map[string]string{},
		Secrets:     map[string]string{},
		Variables:   map[string]string{},
		StepResults: map[string]any{},
	}

	if project != nil {
		runCtx.ProjectID = project.ID
		for _, entry := range project.Constants {
			if entry.Key != "" {
				runCtx.Constants[entry.Key] = entry.Value
			}
		}
		// Variables and secrets both land in Variables so @ref resolves them uniformly.
		for _, entry := range project.Variables {
			if entry.Key != "" {
				runCtx.Variables[entry.Key] = entry.Value
			}
		}
		for _, entry := range project.Secrets {
			if entry.Key != "" {
				runCtx.Secrets[entry.Key] = entry.Value
				runCtx.Variables[entry.Key] = entry.Value
			}
		}
	}
	if definition != nil {
		for _, entry := range definition.Constants {
			if entry.Key != "" {
				runCtx.Constants[entry.Key] = entry.Value
			}
		}
		for _, entry := range definition.Variables {
			if entry.Key != "" {
				runCtx.Variables[entry.Key] = entry.Value
			}
		}
	}

	return runCtx
}
