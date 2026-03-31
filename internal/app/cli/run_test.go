package cliapp

import (
	"testing"

	"github.com/wiregoblin/wiregoblin/internal/model"
)

func TestRedactRunEvent(t *testing.T) {
	t.Parallel()

	event := model.RunEvent{
		Error: "token secret-123 leaked",
		Request: map[string]any{
			"authorization": "Bearer secret-123",
		},
		Response: map[string]any{
			"token": "secret-123",
			"items": []any{"ok", "secret-123"},
		},
	}

	redacted := redactRunEvent(event, []string{"secret-123"})
	if redacted.Error != "token [REDACTED] leaked" {
		t.Fatalf("Error = %q", redacted.Error)
	}
	request := redacted.Request
	if request["authorization"] != "Bearer [REDACTED]" {
		t.Fatalf("authorization = %v", request["authorization"])
	}
	response := redacted.Response.(map[string]any)
	if response["token"] != "[REDACTED]" {
		t.Fatalf("token = %v", response["token"])
	}
	items := response["items"].([]any)
	if items[1] != "[REDACTED]" {
		t.Fatalf("items[1] = %v", items[1])
	}
}
