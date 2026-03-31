package redis

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/wiregoblin/wiregoblin/internal/model"
)

const defaultTimeoutSeconds = 5

// Example YAML config:
//
// address: localhost:6379
// password: @RedisPassword
// db: 0
// command: GET
// args:
//   - session:@SessionID
// timeout_seconds: 5

type redisConfig struct {
	Address string
	//nolint:gosec // Redis AUTH password is runtime configuration, not a hardcoded secret.
	Password       string
	DB             int
	Command        string
	Args           []string
	TimeoutSeconds int
}

func decodeConfig(step model.Step) redisConfig {
	if step.Config == nil {
		step.Config = map[string]any{}
	}
	config := redisConfig{
		DB:             0,
		Command:        "PING",
		Args:           []string{},
		TimeoutSeconds: defaultTimeoutSeconds,
	}
	if v, ok := step.Config["address"].(string); ok {
		config.Address = v
	}
	if v, ok := step.Config["password"].(string); ok {
		config.Password = v
	}
	switch v := step.Config["db"].(type) {
	case float64:
		config.DB = int(v)
	case int:
		config.DB = v
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			config.DB = parsed
		}
	}
	if v, ok := step.Config["command"].(string); ok && strings.TrimSpace(v) != "" {
		config.Command = v
	}
	switch v := step.Config["args"].(type) {
	case []string:
		config.Args = v
	case []any:
		config.Args = make([]string, 0, len(v))
		for _, value := range v {
			config.Args = append(config.Args, fmt.Sprint(value))
		}
	}
	switch v := step.Config["timeout_seconds"].(type) {
	case float64:
		config.TimeoutSeconds = int(v)
	case int:
		config.TimeoutSeconds = v
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			config.TimeoutSeconds = parsed
		}
	}
	return config
}
