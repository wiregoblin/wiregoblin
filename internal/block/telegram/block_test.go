package telegram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/model"
)

func TestReferencePolicyIncludesBaseURL(t *testing.T) {
	policies := New().ReferencePolicy()

	hasPolicy := func(want block.ReferencePolicy) bool {
		for _, policy := range policies {
			if policy == want {
				return true
			}
		}
		return false
	}

	if !hasPolicy(block.ReferencePolicy{Field: "base_url", Constants: true}) {
		t.Fatal("ReferencePolicy() missing base_url constant support")
	}
	if !hasPolicy(block.ReferencePolicy{Field: "parse_mode", Constants: true, Variables: true, InlineOnly: true}) {
		t.Fatal("ReferencePolicy() missing parse_mode reference support")
	}
}

func TestExecuteSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/botsecret/sendMessage" {
			t.Fatalf("Path = %q, want /botsecret/sendMessage", r.URL.Path)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if payload["chat_id"] != "42" {
			t.Fatalf("chat_id = %v", payload["chat_id"])
		}
		if payload["text"] != "hello" {
			t.Fatalf("text = %v", payload["text"])
		}
		if payload["parse_mode"] != "MarkdownV2" {
			t.Fatalf("parse_mode = %v", payload["parse_mode"])
		}

		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer server.Close()

	oldClient := httpClient
	httpClient = server.Client()
	defer func() { httpClient = oldClient }()

	result, err := New().Execute(context.Background(), nil, model.Step{
		Config: map[string]any{
			"base_url":   server.URL,
			"token":      "secret",
			"chat_id":    "42",
			"message":    "hello",
			"parse_mode": "MarkdownV2",
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := result.Output.(map[string]any)
	if output["status"] != 200 {
		t.Fatalf("status = %v, want 200", output["status"])
	}
}

func TestExecuteReturnsErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false})
	}))
	defer server.Close()

	oldClient := httpClient
	httpClient = server.Client()
	defer func() { httpClient = oldClient }()

	result, err := New().Execute(context.Background(), nil, model.Step{
		Config: map[string]any{
			"base_url": server.URL,
			"token":    "secret",
			"chat_id":  "42",
			"message":  "hello",
		},
	})
	if err == nil {
		t.Fatal("Execute() error = nil, want HTTP error")
	}
	if err.Error() != "telegram 400 Bad Request" {
		t.Fatalf("error = %q", err.Error())
	}

	output := result.Output.(map[string]any)
	if output["status"] != 400 {
		t.Fatalf("status = %v, want 400", output["status"])
	}
}

func TestValidateRejectsNegativeTimeout(t *testing.T) {
	err := New().Validate(model.Step{
		Config: map[string]any{
			"token":           "secret",
			"chat_id":         "42",
			"message":         "hello",
			"timeout_seconds": -1,
		},
	})
	if err == nil {
		t.Fatal("Validate() error = nil, want timeout validation error")
	}
}
