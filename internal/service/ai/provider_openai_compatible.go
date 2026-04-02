package ai

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/wiregoblin/wiregoblin/internal/model"
)

func explainOpenAICompatibleWithPrompt(
	ctx context.Context,
	config *model.AIConfig,
	systemPrompt string,
	userPrompt string,
) (string, error) {
	payload := map[string]any{
		"model": config.Model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
	}

	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := doJSONRequest(
		ctx,
		config.TimeoutSeconds,
		http.MethodPost,
		buildOpenAIEndpoint(config.BaseURL),
		payload,
		&response,
	); err != nil {
		return "", err
	}
	if len(response.Choices) == 0 || strings.TrimSpace(response.Choices[0].Message.Content) == "" {
		return "", fmt.Errorf("ai returned an empty response")
	}
	return strings.TrimSpace(response.Choices[0].Message.Content), nil
}

func buildOpenAIEndpoint(baseURL string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	switch {
	case strings.HasSuffix(trimmed, "/chat/completions"):
		return trimmed
	case strings.HasSuffix(trimmed, "/v1"):
		return trimmed + "/chat/completions"
	default:
		return trimmed + "/v1/chat/completions"
	}
}
