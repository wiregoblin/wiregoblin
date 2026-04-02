package ai

import (
	"context"
	"fmt"

	"github.com/wiregoblin/wiregoblin/internal/model"
)

// ExplainFailure requests a concise failed-run explanation from the configured local model.
func ExplainFailure(ctx context.Context, config *model.AIConfig, input DebugInput) (string, error) {
	if config == nil || !config.Enabled {
		return "", fmt.Errorf("ai is not enabled")
	}

	userPrompt, err := buildDebugPrompt(input)
	if err != nil {
		return "", err
	}

	switch config.Provider {
	case "openai_compatible":
		return explainOpenAICompatibleWithPrompt(ctx, config, debugSystemPrompt, userPrompt)
	case "ollama":
		return explainOllamaWithPrompt(ctx, config, debugSystemPrompt, userPrompt)
	default:
		return "", fmt.Errorf("unsupported ai provider %q", config.Provider)
	}
}

// SummarizeSuccess requests a concise successful-run summary from the configured local model.
func SummarizeSuccess(ctx context.Context, config *model.AIConfig, input SuccessInput) (string, error) {
	if config == nil || !config.Enabled {
		return "", fmt.Errorf("ai is not enabled")
	}

	userPrompt, err := buildSuccessPrompt(input)
	if err != nil {
		return "", err
	}

	switch config.Provider {
	case "openai_compatible":
		return explainOpenAICompatibleWithPrompt(ctx, config, successSystemPrompt, userPrompt)
	case "ollama":
		return explainOllamaWithPrompt(ctx, config, successSystemPrompt, userPrompt)
	default:
		return "", fmt.Errorf("unsupported ai provider %q", config.Provider)
	}
}
