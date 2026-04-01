package http //nolint:revive // package name matches the block domain intentionally.

import (
	"context"
	"encoding/json"
	"io"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/wiregoblin/wiregoblin/internal/model"
)

func TestExecuteSuccess(t *testing.T) {
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if r.Method != stdhttp.MethodPost {
			t.Fatalf("Method = %s, want POST", r.Method)
		}
		if r.Header.Get("X-Test") != "value" {
			t.Fatalf("X-Test = %q, want value", r.Header.Get("X-Test"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("Content-Type = %q, want application/json", r.Header.Get("Content-Type"))
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		if string(body) != `{"hello":"world"}` {
			t.Fatalf("body = %q", string(body))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	oldClient := httpClient
	httpClient = server.Client()
	defer func() { httpClient = oldClient }()

	step := model.Step{
		Config: map[string]any{
			"url":     server.URL,
			"method":  stdhttp.MethodPost,
			"body":    `{"hello":"world"}`,
			"headers": map[string]any{"X-Test": "value"},
		},
	}

	result, err := New().Execute(context.Background(), nil, step)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Exports["statusCode"] != "200" {
		t.Fatalf("statusCode = %q, want 200", result.Exports["statusCode"])
	}
	if got := result.Exports["responseTimeMs"]; got == "" {
		t.Fatal("responseTimeMs export is empty")
	}

	output := result.Output.(map[string]any)
	if got, ok := output["responseTimeMs"].(int64); !ok || got < 0 {
		t.Fatalf("responseTimeMs = %#v, want non-negative int64", output["responseTimeMs"])
	}
	if result.Request["method"] != stdhttp.MethodPost {
		t.Fatalf("request.method = %#v, want POST", result.Request["method"])
	}
	if result.Request["url"] != server.URL {
		t.Fatalf("request.url = %#v, want %q", result.Request["url"], server.URL)
	}
	headers := result.Request["headers"].(map[string]any)
	if headers["X-Test"] != "value" {
		t.Fatalf("request.headers[X-Test] = %#v, want value", headers["X-Test"])
	}
	if headers["Content-Type"] != "application/json" {
		t.Fatalf("request.headers[Content-Type] = %#v, want application/json", headers["Content-Type"])
	}
	if result.Request["body"] != `{"hello":"world"}` {
		t.Fatalf("request.body = %#v, want body", result.Request["body"])
	}
	body := output["body"].(map[string]any)
	if body["ok"] != true {
		t.Fatalf("body.ok = %v, want true", body["ok"])
	}
}

func TestExecuteReturnsResponseOnHTTPError(t *testing.T) {
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		w.WriteHeader(stdhttp.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "bad request"})
	}))
	defer server.Close()

	oldClient := httpClient
	httpClient = server.Client()
	defer func() { httpClient = oldClient }()

	result, err := New().Execute(context.Background(), nil, model.Step{
		Config: map[string]any{
			"url":    server.URL,
			"method": stdhttp.MethodGet,
		},
	})
	if err == nil {
		t.Fatal("Execute() error = nil, want HTTP error")
	}
	if err.Error() != "http 400 Bad Request" {
		t.Fatalf("error = %q", err.Error())
	}
	if result.Exports["statusCode"] != "400" {
		t.Fatalf("statusCode = %q, want 400", result.Exports["statusCode"])
	}
	if got := result.Exports["responseTimeMs"]; got == "" {
		t.Fatal("responseTimeMs export is empty")
	}
}

func TestExecuteAllowsPlainTextResponse(t *testing.T) {
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		_, _ = w.Write([]byte("not-json"))
	}))
	defer server.Close()

	oldClient := httpClient
	httpClient = server.Client()
	defer func() { httpClient = oldClient }()

	result, err := New().Execute(context.Background(), nil, model.Step{
		Config: map[string]any{
			"url":    server.URL,
			"method": stdhttp.MethodGet,
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := result.Output.(map[string]any)
	if output["body"] != "not-json" {
		t.Fatalf("body = %#v, want plain text response", output["body"])
	}
}

func TestExecuteRejectsSelfSignedTLSByDefault(t *testing.T) {
	server := httptest.NewTLSServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	oldClient := httpClient
	httpClient = &stdhttp.Client{}
	defer func() { httpClient = oldClient }()

	_, err := New().Execute(context.Background(), nil, model.Step{
		Config: map[string]any{
			"url":    server.URL,
			"method": stdhttp.MethodGet,
		},
	})
	if err == nil {
		t.Fatal("Execute() error = nil, want TLS verification error")
	}
}

func TestExecuteAllowsSelfSignedTLSWhenSkipVerifyEnabled(t *testing.T) {
	server := httptest.NewTLSServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	oldClient := httpClient
	httpClient = &stdhttp.Client{}
	defer func() { httpClient = oldClient }()

	result, err := New().Execute(context.Background(), nil, model.Step{
		Config: map[string]any{
			"url":             server.URL,
			"method":          stdhttp.MethodGet,
			"tls_skip_verify": true,
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := result.Output.(map[string]any)
	body := output["body"].(map[string]any)
	if body["ok"] != true {
		t.Fatalf("body.ok = %v, want true", body["ok"])
	}
}
