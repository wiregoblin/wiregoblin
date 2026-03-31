package transform

import (
	"context"
	"testing"

	"github.com/wiregoblin/wiregoblin/internal/model"
)

func TestExecuteExtractsRegexMatches(t *testing.T) {
	result, err := New().Execute(context.Background(), nil, model.Step{
		Config: map[string]any{
			"value": map[string]any{
				"text": "Your code is 123456",
			},
			"regex": map[string]any{
				"code": map[string]any{
					"from":    "value.text",
					"pattern": `\b(\d{6})\b`,
					"group":   1,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	output := result.Output.(map[string]any)
	extracted := output["extracted"].(map[string]any)
	if got := extracted["code"]; got != "123456" {
		t.Fatalf("code = %q, want 123456", got)
	}
}
