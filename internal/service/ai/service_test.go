package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wiregoblin/wiregoblin/internal/model"
)

func TestExplainFailureOpenAICompatible(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("Path = %q, want /v1/chat/completions", r.URL.Path)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if payload["model"] != "debugger" {
			t.Fatalf("model = %v, want debugger", payload["model"])
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content": "Summary: failed\nLikely cause: boom\nNext checks:\n- inspect logs",
					},
				},
			},
		})
	}))
	defer server.Close()

	oldClient := httpClient
	httpClient = server.Client()
	defer func() { httpClient = oldClient }()

	text, err := ExplainFailure(context.Background(), &model.AIConfig{
		Enabled:  true,
		Provider: "openai_compatible",
		BaseURL:  server.URL,
		Model:    "debugger",
	}, DebugInput{WorkflowID: "wf", Error: "boom"})
	if err != nil {
		t.Fatalf("ExplainFailure() error = %v", err)
	}
	if text == "" {
		t.Fatal("ExplainFailure() returned empty text")
	}
}

func TestExplainFailureOllama(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Fatalf("Path = %q, want /api/chat", r.URL.Path)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if payload["model"] != "debugger" {
			t.Fatalf("model = %v, want debugger", payload["model"])
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": map[string]any{
				"content": "Summary: failed\nLikely cause: upstream timeout\nNext checks:\n- retry",
			},
		})
	}))
	defer server.Close()

	oldClient := httpClient
	httpClient = server.Client()
	defer func() { httpClient = oldClient }()

	text, err := ExplainFailure(context.Background(), &model.AIConfig{
		Enabled:  true,
		Provider: "ollama",
		BaseURL:  server.URL,
		Model:    "debugger",
	}, DebugInput{WorkflowID: "wf", Error: "timeout"})
	if err != nil {
		t.Fatalf("ExplainFailure() error = %v", err)
	}
	if text == "" {
		t.Fatal("ExplainFailure() returned empty text")
	}
}
