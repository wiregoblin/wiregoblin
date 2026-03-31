package retry

import (
	"context"
	"errors"
	"testing"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/model"
)

func TestExecuteRetriesUntilSuccess(t *testing.T) {
	attempts := 0
	builtins := map[string]string{}
	runCtx := &block.RunContext{
		Builtins: builtins,
		ExecuteStep: func(_ context.Context, step model.Step) (*block.Result, error) {
			attempts++
			if step.Type != "http" {
				t.Fatalf("step.Type = %q, want http", step.Type)
			}
			if got := step.Config["url"]; got != "https://example.com" {
				t.Fatalf("url = %v", got)
			}
			if attempts < 3 {
				return &block.Result{Output: map[string]any{"attempt": attempts}}, errors.New("not ready")
			}
			if got := builtins["Retry.Attempt"]; got != "3" {
				t.Fatalf("Retry.Attempt = %q, want 3", got)
			}
			return &block.Result{Output: map[string]any{"status": "ok"}}, nil
		},
	}

	result, err := New().Execute(context.Background(), runCtx, model.Step{
		Name: "wait-for-service",
		Config: map[string]any{
			"max_attempts": 3,
			"delay_ms":     0,
			"block": map[string]any{
				"type": "http",
				"url":  "https://example.com",
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}

	output := result.Output.(map[string]any)
	if got := output["attempts"]; got != 3 {
		t.Fatalf("attempts = %v, want 3", got)
	}
	if got := output["succeeded"]; got != true {
		t.Fatalf("succeeded = %v, want true", got)
	}
}

func TestExecuteReturnsLastFailure(t *testing.T) {
	runCtx := &block.RunContext{
		Builtins: map[string]string{},
		ExecuteStep: func(_ context.Context, _ model.Step) (*block.Result, error) {
			return &block.Result{Output: map[string]any{"status": "down"}}, errors.New("still down")
		},
	}

	result, err := New().Execute(context.Background(), runCtx, model.Step{
		Name: "wait-for-service",
		Config: map[string]any{
			"max_attempts": 2,
			"block": map[string]any{
				"type": "http",
				"url":  "https://example.com",
			},
		},
	})
	if err == nil || err.Error() != "still down" {
		t.Fatalf("error = %v, want still down", err)
	}

	output := result.Output.(map[string]any)
	if got := output["last_error"]; got != "still down" {
		t.Fatalf("last_error = %v, want still down", got)
	}
	if got := output["succeeded"]; got != false {
		t.Fatalf("succeeded = %v, want false", got)
	}
}

func TestExecuteRetriesOnlyConfiguredStatusCodes(t *testing.T) {
	attempts := 0
	runCtx := &block.RunContext{
		Builtins: map[string]string{},
		ExecuteStep: func(_ context.Context, _ model.Step) (*block.Result, error) {
			attempts++
			if attempts < 3 {
				return &block.Result{
					Exports: map[string]string{"statusCode": "503"},
				}, errors.New("http 503")
			}
			return &block.Result{
				Output: map[string]any{"status": "ok"},
			}, nil
		},
	}

	result, err := New().Execute(context.Background(), runCtx, model.Step{
		Name: "retry-http",
		Config: map[string]any{
			"max_attempts": 5,
			"retry_on": map[string]any{
				"status_codes": []any{503},
			},
			"block": map[string]any{
				"type": "http",
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
	if got := result.Output.(map[string]any)["attempts"]; got != 3 {
		t.Fatalf("attempts output = %v, want 3", got)
	}
}

func TestExecuteStopsEarlyOnNonRetryableStatusCode(t *testing.T) {
	attempts := 0
	runCtx := &block.RunContext{
		Builtins: map[string]string{},
		ExecuteStep: func(_ context.Context, _ model.Step) (*block.Result, error) {
			attempts++
			return &block.Result{
				Exports: map[string]string{"statusCode": "404"},
			}, errors.New("http 404")
		},
	}

	result, err := New().Execute(context.Background(), runCtx, model.Step{
		Name: "retry-http",
		Config: map[string]any{
			"max_attempts": 5,
			"retry_on": map[string]any{
				"status_codes": []any{429, 500, 503},
			},
			"block": map[string]any{
				"type": "http",
			},
		},
	})
	if err == nil || err.Error() != "http 404" {
		t.Fatalf("error = %v, want http 404", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}

	output := result.Output.(map[string]any)
	if got := output["retryable"]; got != false {
		t.Fatalf("retryable = %v, want false", got)
	}
	if got := output["stopped_early"]; got != true {
		t.Fatalf("stopped_early = %v, want true", got)
	}
	if got := output["attempts"]; got != 1 {
		t.Fatalf("attempts output = %v, want 1", got)
	}
}

func TestExecuteClearsBuiltinsOnSuccess(t *testing.T) {
	builtins := map[string]string{}
	runCtx := &block.RunContext{
		Builtins: builtins,
		ExecuteStep: func(_ context.Context, _ model.Step) (*block.Result, error) {
			return &block.Result{}, nil
		},
	}

	_, err := New().Execute(context.Background(), runCtx, model.Step{
		Config: map[string]any{
			"max_attempts": 3,
			"block":        map[string]any{"type": "http"},
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if _, ok := builtins["Retry.Attempt"]; ok {
		t.Fatal("Retry.Attempt builtin was not cleared after success")
	}
	if _, ok := builtins["Retry.MaxAttempts"]; ok {
		t.Fatal("Retry.MaxAttempts builtin was not cleared after success")
	}
}

func TestExecuteHonorsTransportErrorsFlag(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		attempts := 0
		runCtx := &block.RunContext{
			Builtins: map[string]string{},
			ExecuteStep: func(_ context.Context, _ model.Step) (*block.Result, error) {
				attempts++
				return nil, errors.New("connection refused")
			},
		}

		_, err := New().Execute(context.Background(), runCtx, model.Step{
			Name: "retry-http",
			Config: map[string]any{
				"max_attempts": 5,
				"retry_on": map[string]any{
					"status_codes": []any{503},
				},
				"block": map[string]any{
					"type": "http",
				},
			},
		})
		if err == nil || err.Error() != "connection refused" {
			t.Fatalf("error = %v, want connection refused", err)
		}
		if attempts != 1 {
			t.Fatalf("attempts = %d, want 1", attempts)
		}
	})

	t.Run("enabled", func(t *testing.T) {
		attempts := 0
		runCtx := &block.RunContext{
			Builtins: map[string]string{},
			ExecuteStep: func(_ context.Context, _ model.Step) (*block.Result, error) {
				attempts++
				if attempts < 3 {
					return nil, errors.New("connection refused")
				}
				return &block.Result{Output: map[string]any{"status": "ok"}}, nil
			},
		}

		_, err := New().Execute(context.Background(), runCtx, model.Step{
			Name: "retry-http",
			Config: map[string]any{
				"max_attempts": 5,
				"retry_on": map[string]any{
					"transport_errors": true,
				},
				"block": map[string]any{
					"type": "http",
				},
			},
		})
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if attempts != 3 {
			t.Fatalf("attempts = %d, want 3", attempts)
		}
	})
}
