package smtp //nolint:revive // package name matches the block domain intentionally.

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/wiregoblin/wiregoblin/internal/model"
)

type smtpConfig struct {
	host           string
	port           int
	username       string
	password       string
	tls            bool
	startTLS       bool
	from           string
	to             []string
	cc             []string
	bcc            []string
	subject        string
	text           string
	html           string
	timeoutSeconds int
}

func decodeConfig(step model.Step) smtpConfig {
	config := smtpConfig{}
	if value, ok := step.Config["host"].(string); ok {
		config.host = strings.TrimSpace(value)
	}
	config.port = decodeInt(step.Config["port"])
	if value, ok := step.Config["username"].(string); ok {
		config.username = value
	}
	if value, ok := step.Config["password"].(string); ok {
		config.password = value
	}
	config.tls = decodeBool(step.Config["tls"])
	config.startTLS = decodeBool(step.Config["starttls"])
	if value, ok := step.Config["from"].(string); ok {
		config.from = strings.TrimSpace(value)
	}
	config.to = decodeStrings(step.Config["to"])
	config.cc = decodeStrings(step.Config["cc"])
	config.bcc = decodeStrings(step.Config["bcc"])
	if value, ok := step.Config["subject"].(string); ok {
		config.subject = value
	}
	if value, ok := step.Config["text"].(string); ok {
		config.text = value
	}
	if value, ok := step.Config["html"].(string); ok {
		config.html = value
	}
	config.timeoutSeconds = decodeInt(step.Config["timeout_seconds"])
	return config
}

func decodeStrings(raw any) []string {
	switch typed := raw.(type) {
	case []string:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			item = strings.TrimSpace(item)
			if item != "" {
				out = append(out, item)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			value := strings.TrimSpace(fmt.Sprint(item))
			if value != "" {
				out = append(out, value)
			}
		}
		return out
	default:
		return nil
	}
}

func decodeInt(raw any) int {
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

func decodeBool(raw any) bool {
	switch typed := raw.(type) {
	case bool:
		return typed
	case string:
		value, err := strconv.ParseBool(strings.TrimSpace(typed))
		if err == nil {
			return value
		}
	}
	return false
}
