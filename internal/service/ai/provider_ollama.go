package ai

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/wiregoblin/wiregoblin/internal/model"
)

func explainOllamaWithPrompt(
	ctx context.Context,
	config *model.AIConfig,
	systemPrompt string,
	userPrompt string,
) (string, error) {
	payload := map[string]any{
		"model":  config.Model,
		"stream": false,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
	}

	var response struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}

	if err := doJSONRequest(
		ctx,
		config.TimeoutSeconds,
		http.MethodPost,
		buildOllamaEndpoint(config.BaseURL),
		payload,
		&response,
	); err != nil {
		return "", err
	}
	if strings.TrimSpace(response.Message.Content) == "" {
		return "", fmt.Errorf("ai returned an empty response")
	}
	return strings.TrimSpace(response.Message.Content), nil
}

func buildOllamaEndpoint(baseURL string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	switch {
	case strings.HasSuffix(trimmed, "/api/chat"):
		return trimmed
	case strings.HasSuffix(trimmed, "/api"):
		return trimmed + "/chat"
	default:
		return trimmed + "/api/chat"
	}
}
