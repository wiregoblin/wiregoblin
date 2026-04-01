// Package cliapp contains CLI-specific application wiring and rendering.
package cliapp

import (
	"context"

	assertblock "github.com/wiregoblin/wiregoblin/internal/block/assert"
	containerblock "github.com/wiregoblin/wiregoblin/internal/block/container"
	delayblock "github.com/wiregoblin/wiregoblin/internal/block/delay"
	foreachblock "github.com/wiregoblin/wiregoblin/internal/block/foreach"
	gotoblock "github.com/wiregoblin/wiregoblin/internal/block/goto"
	grpcblock "github.com/wiregoblin/wiregoblin/internal/block/grpc"
	httpblock "github.com/wiregoblin/wiregoblin/internal/block/http"
	imapblock "github.com/wiregoblin/wiregoblin/internal/block/imap"
	logblock "github.com/wiregoblin/wiregoblin/internal/block/log"
	openaiblock "github.com/wiregoblin/wiregoblin/internal/block/openai"
	parallelblock "github.com/wiregoblin/wiregoblin/internal/block/parallel"
	postgresblock "github.com/wiregoblin/wiregoblin/internal/block/postgres"
	redisblock "github.com/wiregoblin/wiregoblin/internal/block/redis"
	retryblock "github.com/wiregoblin/wiregoblin/internal/block/retry"
	setvarsblock "github.com/wiregoblin/wiregoblin/internal/block/setvars"
	smtpblock "github.com/wiregoblin/wiregoblin/internal/block/smtp"
	telegramblock "github.com/wiregoblin/wiregoblin/internal/block/telegram"
	transformblock "github.com/wiregoblin/wiregoblin/internal/block/transform"
	workflowblock "github.com/wiregoblin/wiregoblin/internal/block/workflow"
	"github.com/wiregoblin/wiregoblin/internal/engine"
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
		service:  workflowservice.New(repo, newRegistry()),
	}
}

// projectID resolves the current project ID from the repository.
func (a *App) projectID(ctx context.Context) (string, error) {
	return a.projects.ProjectID(ctx)
}

// newRegistry builds the block registry used by the workflow engine.
func newRegistry() *engine.Registry {
	registry := engine.NewRegistry()
	registry.Register(assertblock.New())
	registry.Register(containerblock.New())
	registry.Register(delayblock.New())
	registry.Register(foreachblock.New())
	registry.Register(gotoblock.New())
	registry.Register(grpcblock.New())
	registry.Register(httpblock.New())
	registry.Register(imapblock.New())
	registry.Register(logblock.New())
	registry.Register(openaiblock.New())
	registry.Register(parallelblock.New())
	registry.Register(postgresblock.New())
	registry.Register(redisblock.New())
	registry.Register(retryblock.New())
	registry.Register(setvarsblock.New())
	registry.Register(smtpblock.New())
	registry.Register(telegramblock.New())
	registry.Register(transformblock.New())
	registry.Register(workflowblock.New())
	return registry
}
