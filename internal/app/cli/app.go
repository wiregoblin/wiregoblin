// Package cliapp contains CLI-specific application wiring and rendering.
package cliapp

import (
	"github.com/wiregoblin/wiregoblin/internal/models"
	"github.com/wiregoblin/wiregoblin/internal/repository"
	filerepository "github.com/wiregoblin/wiregoblin/internal/repository/file"
	workflowservice "github.com/wiregoblin/wiregoblin/internal/service/workflow"
)

// App wires CLI-oriented application behavior on top of the workflow service.
type App struct {
	projects repository.ProjectRepository
	service  *workflowservice.Service
}

// New creates a CLI application backed by a file-based project repository.
func New(projectPath string) *App {
	repo := filerepository.New(projectPath)
	return &App{
		projects: repo,
		service:  workflowservice.New(repo),
	}
}

// RunWorkflow starts streaming events for one workflow run.
func (a *App) RunWorkflow(workflowName string, opts workflowservice.RunOptions) (<-chan models.RunEvent, error) {
	return a.service.RunWorkflow(workflowName, opts)
}
