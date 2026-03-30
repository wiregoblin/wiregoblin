package telegram

import (
	"strconv"
	"strings"

	"github.com/wiregoblin/wiregoblin/internal/models"
)

// Example YAML config:
//
// token: $TelegramBotToken
// chat_id: "@TelegramChatID"
// message: Deployment finished
// parse_mode: MarkdownV2

type telegramConfig struct {
	BaseURL        string
	Token          string
	ChatID         string
	Message        string
	ParseMode      string
	TimeoutSeconds int
}

func decodeConfig(step models.Step) telegramConfig {
	config := telegramConfig{
		BaseURL: defaultBaseURL,
	}
	if v, ok := step.Config["base_url"].(string); ok && strings.TrimSpace(v) != "" {
		config.BaseURL = strings.TrimSpace(v)
	}
	if v, ok := step.Config["token"].(string); ok {
		config.Token = v
	}
	if v, ok := step.Config["chat_id"].(string); ok {
		config.ChatID = v
	}
	if v, ok := step.Config["message"].(string); ok {
		config.Message = v
	}
	if v, ok := step.Config["parse_mode"].(string); ok {
		config.ParseMode = v
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
