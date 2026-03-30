// Package repository defines application-facing project data access contracts.
package repository

import "github.com/wiregoblin/wiregoblin/internal/models"

// ProjectRepository returns project definitions for the current application context.
type ProjectRepository interface {
	GetProject() (*models.Definition, error)
}
