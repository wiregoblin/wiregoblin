package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wiregoblin/wiregoblin/internal/model"
)

func TestExecuteSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("Path = %q, want /v1/chat/completions", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret-token" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("X-Test") != "value" {
			t.Fatalf("X-Test = %q", r.Header.Get("X-Test"))
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if payload["model"] != "gpt-test" {
			t.Fatalf("model = %v", payload["model"])
		}
		if payload["temperature"] != 0.2 {
			t.Fatalf("temperature = %v, want 0.2", payload["temperature"])
		}
		if payload["max_tokens"] != float64(42) {
			t.Fatalf("max_tokens = %v, want 42", payload["max_tokens"])
		}

		messages := payload["messages"].([]any)
		if len(messages) != 2 {
			t.Fatalf("len(messages) = %d, want 2", len(messages))
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "resp_1",
			"choices": []map[string]any{
				{"message": map[string]any{"content": "hello"}},
			},
		})
	}))
	defer server.Close()

	oldClient := httpClient
	httpClient = server.Client()
	defer func() { httpClient = oldClient }()

	result, err := New().Execute(context.Background(), nil, model.Step{
		Config: map[string]any{
			"base_url":      server.URL,
			"token":         "secret-token",
			"model":         "gpt-test",
			"system_prompt": "system",
			"user_prompt":   "user",
			"temperature":   "0.2",
			"max_tokens":    "42",
			"headers":       map[string]any{"X-Test": "value"},
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := result.Output.(map[string]any)
	if output["status"] != 200 {
		t.Fatalf("status = %v, want 200", output["status"])
	}
	if result.Request["method"] != http.MethodPost {
		t.Fatalf("request.method = %#v, want POST", result.Request["method"])
	}
	headers := result.Request["headers"].(map[string]any)
	if headers["Authorization"] != "Bearer secret-token" {
		t.Fatalf("request.headers.Authorization = %#v", headers["Authorization"])
	}
	body := result.Request["body"].(map[string]any)
	if body["model"] != "gpt-test" {
		t.Fatalf("request.body.model = %#v, want gpt-test", body["model"])
	}
}

func TestExecuteReturnsErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "unauthorized"})
	}))
	defer server.Close()

	oldClient := httpClient
	httpClient = server.Client()
	defer func() { httpClient = oldClient }()

	result, err := New().Execute(context.Background(), nil, model.Step{
		Config: map[string]any{
			"base_url":    server.URL,
			"model":       "gpt-test",
			"user_prompt": "user",
		},
	})
	if err == nil {
		t.Fatal("Execute() error = nil, want HTTP error")
	}
	if err.Error() != "openai 401 Unauthorized" {
		t.Fatalf("error = %q", err.Error())
	}

	output := result.Output.(map[string]any)
	if output["status"] != 401 {
		t.Fatalf("status = %v, want 401", output["status"])
	}
	if result.Request == nil {
		t.Fatal("request = nil, want request for logging")
	}
}

func TestValidateRejectsNegativeTimeout(t *testing.T) {
	err := New().Validate(model.Step{
		Config: map[string]any{
			"base_url":        "https://api.example.com",
			"model":           "gpt-test",
			"user_prompt":     "user",
			"timeout_seconds": -1,
		},
	})
	if err == nil {
		t.Fatal("Validate() error = nil, want timeout validation error")
	}
}
