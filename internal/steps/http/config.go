package http

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/wiregoblin/wiregoblin/internal/models"
)

// Example YAML config:
//
// method: POST
// url: https://api.example.com/v1/items
// body: '{"name":"demo"}'
// headers:
//   Authorization: Bearer $ApiToken

type httpConfig struct {
	Method         string
	URL            string
	Body           string
	Headers        map[string]string
	TimeoutSeconds int
}

func decodeConfig(step models.Step) httpConfig {
	if step.Config == nil {
		step.Config = map[string]any{}
	}
	config := httpConfig{}
	if v, ok := step.Config["method"].(string); ok {
		config.Method = v
	}
	if v, ok := step.Config["url"].(string); ok {
		config.URL = v
	}
	if v, ok := step.Config["body"].(string); ok {
		config.Body = v
	}
	if v, ok := step.Config["headers"].(map[string]string); ok {
		config.Headers = v
	} else if v, ok := step.Config["headers"].(map[string]any); ok {
		config.Headers = make(map[string]string, len(v))
		for k, val := range v {
			config.Headers[k] = fmt.Sprint(val)
		}
	}
	config.TimeoutSeconds = decodeTimeoutSeconds(step.Config["timeout_seconds"])
	return config
}

func decodeTimeoutSeconds(raw any) int {
	switch typed := raw.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		value, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return value
		}
	}
	return 0
}
