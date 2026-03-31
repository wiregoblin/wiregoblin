package transform

import (
	"context"
	"testing"

	"github.com/wiregoblin/wiregoblin/internal/model"
)

func TestExecuteBuildsStructuredValueAndCastsFields(t *testing.T) {
	result, err := New().Execute(context.Background(), nil, model.Step{
		Config: map[string]any{
			"value": map[string]any{
				"id":      "42",
				"active":  "true",
				"profile": "{\"role\":\"admin\"}",
				"items":   []any{"1", "2"},
			},
			"casts": map[string]any{
				"id":      "int",
				"active":  "bool",
				"profile": "json",
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := result.Output.(map[string]any)
	value := output["value"].(map[string]any)
	if got := value["id"]; got != 42 {
		t.Fatalf("id = %v, want 42", got)
	}
	if got := value["active"]; got != true {
		t.Fatalf("active = %v, want true", got)
	}
	profile := value["profile"].(map[string]any)
	if got := profile["role"]; got != "admin" {
		t.Fatalf("role = %v, want admin", got)
	}
}
