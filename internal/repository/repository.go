// Package repository defines application-facing project data access contracts.
package repository

import (
	"context"

	"github.com/wiregoblin/wiregoblin/internal/model"
)

// ProjectRepository returns project definitions for the current application context.
type ProjectRepository interface {
	ProjectID(ctx context.Context) (string, error)
	GetProject(ctx context.Context, projectID string) (*model.Definition, error)
	GetWorkflow(ctx context.Context, projectID string, workflowID string) (*model.Workflow, error)
	ListWorkflows(ctx context.Context, projectID string) ([]string, error)
}
