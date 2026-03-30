package runtime //nolint:revive // package name reflects the runtime layer intentionally.

import (
	"github.com/wiregoblin/wiregoblin/internal/engine"
	assertblock "github.com/wiregoblin/wiregoblin/internal/steps/assert"
	containerblock "github.com/wiregoblin/wiregoblin/internal/steps/container"
	delayblock "github.com/wiregoblin/wiregoblin/internal/steps/delay"
	gotoblock "github.com/wiregoblin/wiregoblin/internal/steps/goto"
	grpcblock "github.com/wiregoblin/wiregoblin/internal/steps/grpc"
	httpblock "github.com/wiregoblin/wiregoblin/internal/steps/http"
	openaiblock "github.com/wiregoblin/wiregoblin/internal/steps/openai"
	postgresblock "github.com/wiregoblin/wiregoblin/internal/steps/postgres"
	redisblock "github.com/wiregoblin/wiregoblin/internal/steps/redis"
	setvarsblock "github.com/wiregoblin/wiregoblin/internal/steps/setvars"
	telegramblock "github.com/wiregoblin/wiregoblin/internal/steps/telegram"
	workflowblock "github.com/wiregoblin/wiregoblin/internal/steps/workflow"
)

// NewRegistry builds the block registry used by the workflow engine.
func NewRegistry() *engine.Registry {
	registry := engine.NewRegistry()
	registry.Register(assertblock.New())
	registry.Register(containerblock.New())
	registry.Register(delayblock.New())
	registry.Register(gotoblock.New())
	registry.Register(grpcblock.New())
	registry.Register(httpblock.New())
	registry.Register(openaiblock.New())
	registry.Register(postgresblock.New())
	registry.Register(redisblock.New())
	registry.Register(setvarsblock.New())
	registry.Register(telegramblock.New())
	registry.Register(workflowblock.New())
	return registry
}
