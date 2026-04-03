package slack

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/model"
)

func TestReferencePolicyIncludesWebhookURL(t *testing.T) {
	policies := New().ReferencePolicy()

	hasPolicy := func(want block.ReferencePolicy) bool {
		for _, policy := range policies {
			if policy == want {
				return true
			}
		}
		return false
	}

	if !hasPolicy(block.ReferencePolicy{Field: "webhook_url", Constants: true}) {
		t.Fatal("ReferencePolicy() missing webhook_url constant support")
	}
	if !hasPolicy(block.ReferencePolicy{Field: "blocks", Constants: true, Variables: true, InlineOnly: true}) {
		t.Fatal("ReferencePolicy() missing blocks reference support")
	}
}

func TestExecuteSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/services/T000/B000/secret" {
			t.Fatalf("Path = %q, want /services/T000/B000/secret", r.URL.Path)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if payload["text"] != "hello" {
			t.Fatalf("text = %v, want hello", payload["text"])
		}
		if payload["channel"] != "#alerts" {
			t.Fatalf("channel = %v, want #alerts", payload["channel"])
		}
		if payload["username"] != "WireGoblin" {
			t.Fatalf("username = %v, want WireGoblin", payload["username"])
		}
		if payload["icon_emoji"] != ":robot_face:" {
			t.Fatalf("icon_emoji = %v, want :robot_face:", payload["icon_emoji"])
		}

		blocks, ok := payload["blocks"].([]any)
		if !ok || len(blocks) != 1 {
			t.Fatalf("blocks = %#v, want one block", payload["blocks"])
		}

		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	oldClient := httpClient
	httpClient = server.Client()
	defer func() { httpClient = oldClient }()

	result, err := New().Execute(context.Background(), nil, model.Step{
		Config: map[string]any{
			"webhook_url": server.URL + "/services/T000/B000/secret",
			"text":        "hello",
			"channel":     "#alerts",
			"username":    "WireGoblin",
			"icon_emoji":  ":robot_face:",
			"blocks":      `[{"type":"section","text":{"type":"mrkdwn","text":"*hi*"}}]`,
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := result.Output.(map[string]any)
	if output["status"] != 200 {
		t.Fatalf("status = %v, want 200", output["status"])
	}
	if output["body"] != "ok" {
		t.Fatalf("body = %#v, want ok", output["body"])
	}
	if result.Request["method"] != http.MethodPost {
		t.Fatalf("request.method = %#v, want POST", result.Request["method"])
	}
	body := result.Request["body"].(map[string]any)
	if body["text"] != "hello" {
		t.Fatalf("request.body.text = %#v, want hello", body["text"])
	}
}

func TestExecuteReturnsErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid_payload"))
	}))
	defer server.Close()

	oldClient := httpClient
	httpClient = server.Client()
	defer func() { httpClient = oldClient }()

	result, err := New().Execute(context.Background(), nil, model.Step{
		Config: map[string]any{
			"webhook_url": server.URL,
			"text":        "hello",
		},
	})
	if err == nil {
		t.Fatal("Execute() error = nil, want HTTP error")
	}
	if err.Error() != "slack 400 Bad Request" {
		t.Fatalf("error = %q", err.Error())
	}

	output := result.Output.(map[string]any)
	if output["status"] != 400 {
		t.Fatalf("status = %v, want 400", output["status"])
	}
	if result.Request == nil {
		t.Fatal("request = nil, want request for logging")
	}
}

func TestValidateRejectsInvalidConfig(t *testing.T) {
	blk := New()

	err := blk.Validate(model.Step{
		Config: map[string]any{
			"webhook_url": "",
			"text":        "hello",
		},
	})
	if err == nil {
		t.Fatal("Validate() error = nil, want webhook validation error")
	}

	err = blk.Validate(model.Step{
		Config: map[string]any{
			"webhook_url": "https://hooks.slack.test/services/T/B/secret",
			"blocks":      "not-json",
		},
	})
	if err == nil {
		t.Fatal("Validate() error = nil, want blocks json validation error")
	}

	err = blk.Validate(model.Step{
		Config: map[string]any{
			"webhook_url":     "https://hooks.slack.test/services/T/B/secret",
			"text":            "hello",
			"timeout_seconds": -1,
		},
	})
	if err == nil {
		t.Fatal("Validate() error = nil, want timeout validation error")
	}
}
