package slack

import (
	"strconv"
	"strings"

	"github.com/wiregoblin/wiregoblin/internal/model"
)

// Example YAML config:
//
// webhook_url: @SlackWebhookURL
// text: Deployment finished
// channel: "#deployments"
// username: WireGoblin
// icon_emoji: ":robot_face:"
// blocks: '[{"type":"section","text":{"type":"mrkdwn","text":"*Deploy finished*"}}]'

type slackConfig struct {
	WebhookURL     string
	Text           string
	Channel        string
	Username       string
	IconEmoji      string
	Blocks         string
	TimeoutSeconds int
}

func decodeConfig(step model.Step) slackConfig {
	config := slackConfig{}
	if v, ok := step.Config["webhook_url"].(string); ok {
		config.WebhookURL = strings.TrimSpace(v)
	}
	if v, ok := step.Config["text"].(string); ok {
		config.Text = v
	}
	if v, ok := step.Config["channel"].(string); ok {
		config.Channel = v
	}
	if v, ok := step.Config["username"].(string); ok {
		config.Username = v
	}
	if v, ok := step.Config["icon_emoji"].(string); ok {
		config.IconEmoji = v
	}
	if v, ok := step.Config["blocks"].(string); ok {
		config.Blocks = v
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
