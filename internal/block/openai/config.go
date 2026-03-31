package openai

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/wiregoblin/wiregoblin/internal/model"
)

// Example YAML config:
//
// base_url: https://api.openai.com/v1
// token: @OpenAIToken
// model: gpt-4.1-mini
// system_prompt: You are a concise assistant.
// user_prompt: "$prompt"
// temperature: "0.2"
// max_tokens: "300"

type openaiConfig struct {
	BaseURL        string
	Token          string
	Model          string
	SystemPrompt   string
	UserPrompt     string
	Temperature    string
	MaxTokens      string
	Headers        map[string]string
	TimeoutSeconds int
}

func decodeConfig(step model.Step) openaiConfig {
	config := openaiConfig{}
	if v, ok := step.Config["base_url"].(string); ok {
		config.BaseURL = v
	}
	if v, ok := step.Config["token"].(string); ok {
		config.Token = v
	}
	if v, ok := step.Config["model"].(string); ok {
		config.Model = v
	}
	if v, ok := step.Config["system_prompt"].(string); ok {
		config.SystemPrompt = v
	}
	if v, ok := step.Config["user_prompt"].(string); ok {
		config.UserPrompt = v
	}
	if v, ok := step.Config["temperature"].(string); ok {
		config.Temperature = v
	}
	if v, ok := step.Config["max_tokens"].(string); ok {
		config.MaxTokens = v
	}
	if v, ok := step.Config["headers"].(map[string]string); ok {
		config.Headers = v
	} else if v, ok := step.Config["headers"].(map[string]any); ok {
		config.Headers = make(map[string]string, len(v))
		for key, value := range v {
			config.Headers[key] = fmt.Sprint(value)
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
