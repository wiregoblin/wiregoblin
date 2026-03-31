package imap

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/wiregoblin/wiregoblin/internal/model"
)

type criteriaConfig struct {
	MessageID       string
	From            string
	To              string
	SubjectContains string
	BodyContains    string
	UnseenOnly      bool
}

type waitConfig struct {
	TimeoutMS      int
	PollIntervalMS int
}

type imapConfig struct {
	host           string
	port           int
	username       string
	password       string
	tls            bool
	mailbox        string
	criteria       criteriaConfig
	wait           waitConfig
	selectMode     string
	markAsSeen     bool
	delete         bool
	timeoutSeconds int
}

func decodeConfig(step model.Step) imapConfig {
	config := imapConfig{
		mailbox:    "INBOX",
		selectMode: "latest",
		wait: waitConfig{
			PollIntervalMS: 1000,
		},
	}
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
	if value, ok := step.Config["mailbox"].(string); ok && strings.TrimSpace(value) != "" {
		config.mailbox = strings.TrimSpace(value)
	}
	config.criteria = decodeCriteria(step.Config["criteria"])
	config.wait = decodeWait(step.Config["wait"], config.wait)
	if value, ok := step.Config["select"].(string); ok && strings.TrimSpace(value) != "" {
		config.selectMode = strings.ToLower(strings.TrimSpace(value))
	}
	config.markAsSeen = decodeBool(step.Config["mark_as_seen"])
	config.delete = decodeBool(step.Config["delete"])
	config.timeoutSeconds = decodeInt(step.Config["timeout_seconds"])
	return config
}

func decodeCriteria(raw any) criteriaConfig {
	config := criteriaConfig{}
	m, ok := raw.(map[string]any)
	if !ok {
		return config
	}
	if value, ok := m["from"].(string); ok {
		config.From = value
	}
	if value, ok := m["message_id"].(string); ok {
		config.MessageID = value
	}
	if value, ok := m["to"].(string); ok {
		config.To = value
	}
	if value, ok := m["subject_contains"].(string); ok {
		config.SubjectContains = value
	}
	if value, ok := m["body_contains"].(string); ok {
		config.BodyContains = value
	}
	config.UnseenOnly = decodeBool(m["unseen_only"])
	return config
}

func decodeWait(raw any, defaults waitConfig) waitConfig {
	config := defaults
	m, ok := raw.(map[string]any)
	if !ok {
		return config
	}
	if value := decodeInt(m["timeout_ms"]); value > 0 {
		config.TimeoutMS = value
	}
	if value := decodeInt(m["poll_interval_ms"]); value > 0 {
		config.PollIntervalMS = value
	}
	return config
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

func (c imapConfig) validate() error {
	if c.host == "" {
		return fmt.Errorf("imap host is required")
	}
	if c.port <= 0 {
		return fmt.Errorf("imap port must be greater than 0")
	}
	if c.username == "" {
		return fmt.Errorf("imap username is required")
	}
	if c.password == "" {
		return fmt.Errorf("imap password is required")
	}
	if c.mailbox == "" {
		return fmt.Errorf("imap mailbox is required")
	}
	if c.selectMode != "latest" && c.selectMode != "first" {
		return fmt.Errorf("imap select must be latest or first")
	}
	if c.timeoutSeconds < 0 {
		return fmt.Errorf("imap timeout_seconds must be non-negative")
	}
	if c.wait.TimeoutMS < 0 || c.wait.PollIntervalMS < 0 {
		return fmt.Errorf("imap wait values must be non-negative")
	}
	return nil
}
