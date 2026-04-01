package retry

import (
	"context"
	"errors"
	"testing"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/model"
)

func TestExecuteRetriesUntilSuccessWithoutRetryRules(t *testing.T) {
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
				t.Fatalf("url = %v, want https://example.com", got)
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
		t.Fatalf("attempts output = %v, want 3", got)
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

func TestExecuteRetriesConfiguredStatusCode(t *testing.T) {
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
				"match": "any",
				"rules": []any{
					map[string]any{
						"type": "status_code",
						"in":   []any{503},
					},
				},
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

	history, ok := result.Output.(map[string]any)["history"].([]map[string]any)
	if !ok {
		t.Fatalf("history = %#v, want []map[string]any", result.Output.(map[string]any)["history"])
	}
	if len(history) != 3 {
		t.Fatalf("len(history) = %d, want 3", len(history))
	}
	if got := history[0]["attempt"]; got != 1 {
		t.Fatalf("history[0].attempt = %v, want 1", got)
	}
	if got := history[0]["retryable"]; got != true {
		t.Fatalf("history[0].retryable = %v, want true", got)
	}
	if got := history[2]["retryable"]; got != false {
		t.Fatalf("history[2].retryable = %v, want false", got)
	}
}

func TestExecuteStopsEarlyOnNonMatchingStatusCode(t *testing.T) {
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
				"match": "any",
				"rules": []any{
					map[string]any{
						"type": "status_code",
						"in":   []any{429, 500, 503},
					},
				},
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
}

func TestExecuteRetriesTransportErrorWhenConfigured(t *testing.T) {
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
				"match": "any",
				"rules": []any{
					map[string]any{"type": "transport_error"},
				},
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
}

func TestExecuteRetriesPathRuleUntilBodyDataIsNotEmpty(t *testing.T) {
	attempts := 0
	runCtx := &block.RunContext{
		Builtins: map[string]string{},
		ExecuteStep: func(_ context.Context, _ model.Step) (*block.Result, error) {
			attempts++
			if attempts < 3 {
				return &block.Result{
					Output: map[string]any{
						"body": map[string]any{
							"data": "",
						},
					},
				}, nil
			}
			return &block.Result{
				Output: map[string]any{
					"body": map[string]any{
						"data": "ready",
					},
				},
			}, nil
		},
	}

	result, err := New().Execute(context.Background(), runCtx, model.Step{
		Name: "retry-grpc",
		Config: map[string]any{
			"max_attempts": 5,
			"retry_on": map[string]any{
				"match": "any",
				"rules": []any{
					map[string]any{
						"type":     "path",
						"path":     "body.data",
						"operator": "empty",
					},
				},
			},
			"block": map[string]any{
				"type": "grpc",
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
	if got := result.Output.(map[string]any)["succeeded"]; got != true {
		t.Fatalf("succeeded = %v, want true", got)
	}
}

func TestExecuteRetriesPathRuleForArrayLength(t *testing.T) {
	attempts := 0
	runCtx := &block.RunContext{
		Builtins: map[string]string{},
		ExecuteStep: func(_ context.Context, _ model.Step) (*block.Result, error) {
			attempts++
			if attempts < 3 {
				return &block.Result{
					Output: map[string]any{
						"body": map[string]any{
							"other_list": []any{},
						},
					},
				}, nil
			}
			return &block.Result{
				Output: map[string]any{
					"body": map[string]any{
						"other_list": []any{"item"},
					},
				},
			}, nil
		},
	}

	_, err := New().Execute(context.Background(), runCtx, model.Step{
		Name: "retry-grpc",
		Config: map[string]any{
			"max_attempts": 5,
			"retry_on": map[string]any{
				"match": "any",
				"rules": []any{
					map[string]any{
						"type":     "path",
						"path":     "body.other_list.length",
						"operator": "=",
						"expected": 0,
					},
				},
			},
			"block": map[string]any{
				"type": "grpc",
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}

func TestExecuteFailsWhenRetryBudgetIsExhaustedWithoutNestedError(t *testing.T) {
	attempts := 0
	runCtx := &block.RunContext{
		Builtins: map[string]string{},
		ExecuteStep: func(_ context.Context, _ model.Step) (*block.Result, error) {
			attempts++
			return &block.Result{
				Output: map[string]any{
					"body": map[string]any{
						"data": "",
					},
				},
			}, nil
		},
	}

	result, err := New().Execute(context.Background(), runCtx, model.Step{
		Name: "retry-grpc",
		Config: map[string]any{
			"max_attempts": 3,
			"retry_on": map[string]any{
				"match": "any",
				"rules": []any{
					map[string]any{
						"type":     "path",
						"path":     "body.data",
						"operator": "empty",
					},
				},
			},
			"block": map[string]any{
				"type": "grpc",
			},
		},
	})
	if err == nil || err.Error() != "retry exhausted after 3 attempts" {
		t.Fatalf("error = %v, want retry exhausted after 3 attempts", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}

	output := result.Output.(map[string]any)
	if got := output["succeeded"]; got != false {
		t.Fatalf("succeeded = %v, want false", got)
	}
	if got := output["last_error"]; got != "retry exhausted after 3 attempts" {
		t.Fatalf("last_error = %v, want retry exhausted after 3 attempts", got)
	}
}

func TestExecuteEmitsAttemptProgressEvents(t *testing.T) {
	started := make([]string, 0)
	finished := make([]model.StepFinishEvent, 0)
	runCtx := &block.RunContext{
		Builtins: map[string]string{},
		ExecuteStep: func(_ context.Context, _ model.Step) (*block.Result, error) {
			return &block.Result{
				Output:  map[string]any{"status": "ok"},
				Request: map[string]any{"url": "https://example.com/final"},
			}, nil
		},
		EmitStepStart: func(event model.StepStartEvent) {
			started = append(started, event.Step.Name)
		},
		EmitStepFinish: func(event model.StepFinishEvent) {
			finished = append(finished, event)
		},
	}

	_, err := New().Execute(context.Background(), runCtx, model.Step{
		Name: "retry-http",
		Config: map[string]any{
			"max_attempts": 1,
			"block": map[string]any{
				"name": "Nested HTTP",
				"type": "http",
				"url":  "https://example.com/$user_id",
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(started) != 1 || started[0] != "Nested HTTP (attempt 1/1)" {
		t.Fatalf("started = %#v, want nested retry attempt name", started)
	}
	if len(finished) != 1 {
		t.Fatalf("len(finished) = %d, want 1", len(finished))
	}
	if finished[0].Status != "ok" {
		t.Fatalf("finish status = %q, want ok", finished[0].Status)
	}
	if finished[0].Duration < 0 {
		t.Fatalf("finish duration = %v, want >= 0", finished[0].Duration)
	}
	if finished[0].Request["url"] != "https://example.com/final" {
		t.Fatalf("finish request url = %#v, want final url", finished[0].Request["url"])
	}
}

func TestExecuteRetriesWhenAllRulesMatch(t *testing.T) {
	attempts := 0
	runCtx := &block.RunContext{
		Builtins: map[string]string{},
		ExecuteStep: func(_ context.Context, _ model.Step) (*block.Result, error) {
			attempts++
			if attempts == 1 {
				return &block.Result{
					Output: map[string]any{
						"body": map[string]any{
							"data": "",
						},
					},
					Exports: map[string]string{"statusCode": "503"},
				}, errors.New("http 503")
			}
			return &block.Result{
				Output: map[string]any{
					"body": map[string]any{
						"data": "ready",
					},
				},
				Exports: map[string]string{"statusCode": "200"},
			}, nil
		},
	}

	_, err := New().Execute(context.Background(), runCtx, model.Step{
		Name: "retry-http",
		Config: map[string]any{
			"max_attempts": 3,
			"retry_on": map[string]any{
				"match": "all",
				"rules": []any{
					map[string]any{
						"type": "status_code",
						"in":   []any{503},
					},
					map[string]any{
						"type":     "path",
						"path":     "body.data",
						"operator": "empty",
					},
				},
			},
			"block": map[string]any{
				"type": "http",
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
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

func TestValidateRejectsInvalidRetryOn(t *testing.T) {
	err := New().Validate(model.Step{
		Config: map[string]any{
			"max_attempts": 3,
			"retry_on": map[string]any{
				"match": "sometimes",
				"rules": []any{
					map[string]any{"type": "transport_error"},
				},
			},
			"block": map[string]any{"type": "http"},
		},
	})
	if err == nil || err.Error() != "retry retry_on match must be any or all" {
		t.Fatalf("Validate() error = %v, want invalid match", err)
	}
}
