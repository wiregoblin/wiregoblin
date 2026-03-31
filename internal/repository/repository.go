// Package repository defines application-facing project data access contracts.
package repository

import (
	"context"

	"github.com/wiregoblin/wiregoblin/internal/model"
)

// ProjectRepository returns project definitions for the current application context.
type ProjectRepository interface {
	GetProject(ctx context.Context) (*model.Definition, error)
}
