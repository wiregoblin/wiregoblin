package grpc

import (
	"fmt"
	"strings"

	"github.com/wiregoblin/wiregoblin/internal/model"
)

// Example YAML config:
//
// address: localhost:50051
// tls: false
// method: /package.Service/Method
// request: "{\"user_id\":\"$UserID\"}"
// metadata:
//   authorization: Bearer @Token

// Config is the persisted config for a gRPC workflow block.
type Config struct {
	Address  string            `json:"address"`
	TLS      bool              `json:"tls"`
	Method   string            `json:"method"`
	Request  string            `json:"request"`
	Metadata map[string]string `json:"metadata"`
}

func decodeConfig(step model.Step) Config {
	if step.Config == nil {
		step.Config = map[string]any{}
	}

	config := Config{}

	if raw, ok := step.Config["address"].(string); ok {
		config.Address = raw
	}
	switch raw := step.Config["tls"].(type) {
	case bool:
		config.TLS = raw
	case string:
		config.TLS = strings.EqualFold(strings.TrimSpace(raw), "true")
	}
	if raw, ok := step.Config["method"].(string); ok {
		config.Method = raw
	}
	if raw, ok := step.Config["request"].(string); ok {
		config.Request = raw
	}
	if raw, ok := step.Config["metadata"].(map[string]string); ok {
		config.Metadata = cloneStrings(raw)
	} else if raw, ok := step.Config["metadata"].(map[string]any); ok {
		config.Metadata = convertStringMap(raw)
	}

	if config.Metadata == nil {
		config.Metadata = map[string]string{}
	}

	return config
}

func convertStringMap(raw map[string]any) map[string]string {
	values := make(map[string]string, len(raw))
	for key, value := range raw {
		values[key] = fmt.Sprint(value)
	}
	return values
}

func cloneStrings(values map[string]string) map[string]string {
	if values == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}
