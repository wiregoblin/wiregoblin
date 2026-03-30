// Package steps defines stable workflow block type identifiers.
package steps

// Stable workflow block type identifiers.
const (
	BlockTypeGRPC      = "grpc"
	BlockTypeHTTP      = "http"
	BlockTypePostgres  = "postgres"
	BlockTypeRedis     = "redis"
	BlockTypeOpenAI    = "openai"
	BlockTypeTelegram  = "telegram"
	BlockTypeAssert    = "assert"
	BlockTypeGoto      = "goto"
	BlockTypeWorkflow  = "workflow"
	BlockTypeContainer = "container"
	BlockTypeDelay     = "delay"
	BlockTypeSetVars   = "setvars"
)
