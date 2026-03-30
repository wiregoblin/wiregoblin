package grpc

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	blockpkg "github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/models"
)

func TestExecuteParsesJSONResponse(t *testing.T) {
	step := models.Step{
		Config: map[string]any{
			"address": "localhost:50051",
			"method":  "/svc.Method",
			"request": `{"user_id":"42"}`,
		},
	}

	block := &Block{
		invoke: func(_ context.Context, config Config) (string, error) {
			if config.Address != "localhost:50051" {
				t.Fatalf("Address = %q", config.Address)
			}
			if config.TLS {
				t.Fatal("TLS = true, want false")
			}
			if config.Method != "/svc.Method" {
				t.Fatalf("Method = %q", config.Method)
			}
			if config.Request != `{"user_id":"42"}` {
				t.Fatalf("Request = %q", config.Request)
			}
			return `{"ok":true,"count":2}`, nil
		},
	}

	result, err := block.Execute(context.Background(), nil, step)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := result.Output.(map[string]any)
	if output["ok"] != true {
		t.Fatalf("ok = %v, want true", output["ok"])
	}
	if output["count"] != float64(2) {
		t.Fatalf("count = %v, want 2", output["count"])
	}
}

func TestExecuteParsesTLSFlag(t *testing.T) {
	block := &Block{
		invoke: func(_ context.Context, config Config) (string, error) {
			if !config.TLS {
				t.Fatal("TLS = false, want true")
			}
			return `{"ok":true}`, nil
		},
	}

	_, err := block.Execute(context.Background(), nil, models.Step{
		Config: map[string]any{
			"address": "demo.connectrpc.com:443",
			"tls":     true,
			"method":  "/connectrpc.eliza.v1.ElizaService/Say",
			"request": `{"sentence":"hello"}`,
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestExecuteResolvesVariablesAndConstantsInRequestJSON(t *testing.T) {
	grpcBlock := &Block{
		invoke: func(_ context.Context, config Config) (string, error) {
			var request map[string]any
			if err := json.Unmarshal([]byte(config.Request), &request); err != nil {
				t.Fatalf("Unmarshal(request) error = %v", err)
			}
			if request["user_id"] != "42" || request["token"] != "secret" {
				t.Fatalf("Request = %#v", request)
			}
			nested, ok := request["nested"].(map[string]any)
			if !ok || nested["name"] != "demo" {
				t.Fatalf("nested = %#v", request["nested"])
			}
			tags, ok := request["tags"].([]any)
			if !ok || len(tags) != 3 || tags[0] != "42" || tags[1] != "secret" || tags[2] != "static" {
				t.Fatalf("tags = %#v", request["tags"])
			}
			return `{"ok":true}`, nil
		},
	}

	_, err := grpcBlock.Execute(context.Background(), &blockpkg.RunContext{
		Variables: map[string]string{"UserID": "42", "Name": "demo"},
		Constants: map[string]string{"Token": "secret"},
	}, models.Step{
		Config: map[string]any{
			"address": "localhost:50051",
			"method":  "/svc.Method",
			"request": `{"user_id":"@UserID","token":"$Token","nested":{"name":"@Name"},"tags":["@UserID","$Token","static"]}`,
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestExecuteReturnsRawResponseWhenNotJSON(t *testing.T) {
	block := &Block{
		invoke: func(context.Context, Config) (string, error) {
			return "plain-text", nil
		},
	}

	result, err := block.Execute(context.Background(), nil, models.Step{
		Config: map[string]any{
			"address": "localhost:50051",
			"method":  "/svc.Method",
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Output != "plain-text" {
		t.Fatalf("Output = %#v, want %q", result.Output, "plain-text")
	}
}

func TestExecuteReturnsInvokeError(t *testing.T) {
	wantErr := errors.New("invoke failed")
	block := &Block{
		invoke: func(context.Context, Config) (string, error) {
			return "", wantErr
		},
	}

	_, err := block.Execute(context.Background(), nil, models.Step{
		Config: map[string]any{
			"address": "localhost:50051",
			"method":  "/svc.Method",
		},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
}
