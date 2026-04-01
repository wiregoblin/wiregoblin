package engine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/wiregoblin/wiregoblin/internal/block"
	foreachblock "github.com/wiregoblin/wiregoblin/internal/block/foreach"
	retryblock "github.com/wiregoblin/wiregoblin/internal/block/retry"
	transformblock "github.com/wiregoblin/wiregoblin/internal/block/transform"
	workflowblock "github.com/wiregoblin/wiregoblin/internal/block/workflow"
	"github.com/wiregoblin/wiregoblin/internal/model"
)

type nopObserver struct{}

func (nopObserver) OnStepStart(model.StepStartEvent)   {}
func (nopObserver) OnStepFinish(model.StepFinishEvent) {}

type observerCall struct {
	start  *model.StepStartEvent
	finish *model.StepFinishEvent
}

type captureObserver struct {
	calls []observerCall
}

func (o *captureObserver) OnStepStart(event model.StepStartEvent) {
	o.calls = append(o.calls, observerCall{start: &event})
}

func (o *captureObserver) OnStepFinish(event model.StepFinishEvent) {
	for i := range o.calls {
		if o.calls[i].start != nil && o.calls[i].finish == nil {
			o.calls[i].finish = &event
			return
		}
	}
	o.calls = append(o.calls, observerCall{finish: &event})
}

type fakeBlock struct {
	typ             model.BlockType
	validateErr     error
	executeErr      error
	output          any
	execute         func(ctx context.Context, runCtx *block.RunContext, step model.Step) (*block.Result, error)
	referencePolicy []block.ReferencePolicy
	responseMapping bool
}

func (b *fakeBlock) Type() model.BlockType {
	return b.typ
}

func (b *fakeBlock) Validate(_ model.Step) error {
	return b.validateErr
}

func (b *fakeBlock) Execute(ctx context.Context, runCtx *block.RunContext, step model.Step) (*block.Result, error) {
	if b.execute != nil {
		return b.execute(ctx, runCtx, step)
	}
	if b.executeErr != nil {
		return nil, b.executeErr
	}
	if b.output != nil {
		return &block.Result{Output: b.output}, nil
	}
	return &block.Result{Output: step.Config}, nil
}

func (b *fakeBlock) ReferencePolicy() []block.ReferencePolicy {
	return b.referencePolicy
}

func (b *fakeBlock) SupportsResponseMapping() bool {
	return b.responseMapping
}

func TestRunReturnsStepError(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&fakeBlock{
		typ:        "fake",
		executeErr: errors.New("boom"),
	})

	runner := New(registry, nopObserver{})
	results, err := runner.Run(context.Background(), nil, &model.Workflow{
		Name: "test",
		Steps: []model.Step{{
			Name:    "step",
			Type:    "fake",
			Enabled: true,
			Config:  map[string]any{},
		}},
	})
	if err == nil || err.Error() != "boom" {
		t.Fatalf("expected execution error, got %v", err)
	}
	if len(results) != 1 || results[0].Status != "failed" {
		t.Fatalf("expected one failed result, got %#v", results)
	}
}

func TestRunRejectsUnsupportedAssignMappings(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&fakeBlock{
		typ:             "fake",
		responseMapping: false,
	})

	runner := New(registry, nopObserver{})
	_, err := runner.Run(context.Background(), nil, &model.Workflow{
		Name: "test",
		Steps: []model.Step{{
			Name:    "step",
			Type:    "fake",
			Enabled: true,
			Config: map[string]any{
				"assign": map[string]any{"$value": "body.id"},
			},
		}},
	})
	if err == nil || err.Error() != `block type "fake" does not support assign mappings` {
		t.Fatalf("expected unsupported assign mappings error, got %v", err)
	}
}

func TestRunResolvesOnlyAllowedFields(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&fakeBlock{
		typ: "fake",
		referencePolicy: []block.ReferencePolicy{
			{Field: "allowed", Constants: true, Variables: true, InlineOnly: true},
		},
	})

	runner := New(registry, nopObserver{})
	results, err := runner.Run(context.Background(), &model.Project{
		Variables: []model.Entry{{Key: "name", Value: "resolved"}},
	}, &model.Workflow{
		Name: "test",
		Steps: []model.Step{{
			Name:    "step",
			Type:    "fake",
			Enabled: true,
			Config: map[string]any{
				"allowed": "$name",
				"denied":  "$name",
			},
		}},
	})
	if err != nil {
		t.Fatalf("expected successful run, got %v", err)
	}

	output, ok := results[0].Output.(map[string]any)
	if !ok {
		t.Fatalf("expected map output, got %#v", results[0].Output)
	}
	if got := output["allowed"]; got != "resolved" {
		t.Fatalf("expected allowed field to be resolved, got %v", got)
	}
	if got := output["denied"]; got != "$name" {
		t.Fatalf("expected denied field to stay unresolved, got %v", got)
	}
}

func TestRunResolvesConstantsInAllowedFields(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&fakeBlock{
		typ: "fake",
		referencePolicy: []block.ReferencePolicy{
			{Field: "url", Constants: true, InlineOnly: true},
		},
	})

	runner := New(registry, nopObserver{})
	results, err := runner.Run(context.Background(), &model.Project{
		Constants: []model.Entry{{Key: "users_url", Value: "https://example.com/users/1"}},
	}, &model.Workflow{
		Name: "test",
		Steps: []model.Step{{
			Name:    "step",
			Type:    "fake",
			Enabled: true,
			Config: map[string]any{
				"url": "@users_url",
			},
		}},
	})
	if err != nil {
		t.Fatalf("expected successful run, got %v", err)
	}

	output, ok := results[0].Output.(map[string]any)
	if !ok {
		t.Fatalf("expected map output, got %#v", results[0].Output)
	}
	if got := output["url"]; got != "https://example.com/users/1" {
		t.Fatalf("expected url to be resolved, got %v", got)
	}
}

func TestRunInterpolatesInlineReferencesInAllowedFields(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&fakeBlock{
		typ: "fake",
		referencePolicy: []block.ReferencePolicy{
			{Field: "authorization", Constants: true, Variables: true, InlineOnly: true},
		},
	})

	runner := New(registry, nopObserver{})
	results, err := runner.Run(context.Background(), &model.Project{
		Constants: []model.Entry{{Key: "scheme", Value: "Bearer"}},
		Variables: []model.Entry{{Key: "token", Value: "secret-token"}},
	}, &model.Workflow{
		Name: "test",
		Steps: []model.Step{{
			Name:    "step",
			Type:    "fake",
			Enabled: true,
			Config: map[string]any{
				"authorization": "@scheme $token",
			},
		}},
	})
	if err != nil {
		t.Fatalf("expected successful run, got %v", err)
	}

	output, ok := results[0].Output.(map[string]any)
	if !ok {
		t.Fatalf("expected map output, got %#v", results[0].Output)
	}
	if got := output["authorization"]; got != "Bearer secret-token" {
		t.Fatalf("expected inline references to resolve, got %v", got)
	}
}

func TestRunResolvesSecretsThroughDollarReferences(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&fakeBlock{
		typ: "fake",
		referencePolicy: []block.ReferencePolicy{
			{Field: "authorization", Constants: true, InlineOnly: true},
		},
	})

	runner := New(registry, nopObserver{})
	results, err := runner.Run(context.Background(), &model.Project{
		Secrets: []model.Entry{{Key: "token", Value: "secret-token"}},
	}, &model.Workflow{
		Name: "test",
		Steps: []model.Step{{
			Name:    "step",
			Type:    "fake",
			Enabled: true,
			Config: map[string]any{
				"authorization": "Bearer @token",
			},
		}},
	})
	if err != nil {
		t.Fatalf("expected successful run, got %v", err)
	}

	output, ok := results[0].Output.(map[string]any)
	if !ok {
		t.Fatalf("expected map output, got %#v", results[0].Output)
	}
	if got := output["authorization"]; got != "Bearer [REDACTED]" {
		t.Fatalf("expected secret to resolve through @, got %v", got)
	}
}

func TestRunDoesNotResolveSecretsThroughVariableNamespace(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&fakeBlock{
		typ: "fake",
		referencePolicy: []block.ReferencePolicy{
			{Field: "authorization", Variables: true, InlineOnly: true},
		},
	})

	runner := New(registry, nopObserver{})
	results, err := runner.Run(context.Background(), &model.Project{
		Secrets: []model.Entry{{Key: "token", Value: "secret-token"}},
	}, &model.Workflow{
		Name: "test",
		Steps: []model.Step{{
			Name:    "step",
			Type:    "fake",
			Enabled: true,
			Config: map[string]any{
				"authorization": "Bearer $token",
			},
		}},
	})
	if err != nil {
		t.Fatalf("expected successful run, got %v", err)
	}

	output, ok := results[0].Output.(map[string]any)
	if !ok {
		t.Fatalf("expected map output, got %#v", results[0].Output)
	}
	if got := output["authorization"]; got != "Bearer $token" {
		t.Fatalf("expected secret reference to remain unresolved, got %v", got)
	}
}

func TestRunResolvesSecretVariablesThroughVariableNamespace(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&fakeBlock{
		typ: "fake",
		referencePolicy: []block.ReferencePolicy{
			{Field: "authorization", Variables: true, InlineOnly: true},
		},
	})

	runner := New(registry, nopObserver{})
	results, err := runner.Run(context.Background(), &model.Project{
		SecretVariables: []model.Entry{{Key: "token", Value: "secret-token"}},
	}, &model.Workflow{
		Name: "test",
		Steps: []model.Step{{
			Name:    "step",
			Type:    "fake",
			Enabled: true,
			Config: map[string]any{
				"authorization": "Bearer $token",
			},
		}},
	})
	if err != nil {
		t.Fatalf("expected successful run, got %v", err)
	}

	output, ok := results[0].Output.(map[string]any)
	if !ok {
		t.Fatalf("expected map output, got %#v", results[0].Output)
	}
	if got := output["authorization"]; got != "Bearer [REDACTED]" {
		t.Fatalf("expected secret variable to resolve through $, got %v", got)
	}
}

func TestRunDoesNotTreatDotAsPartOfReferenceName(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&fakeBlock{
		typ: "fake",
		referencePolicy: []block.ReferencePolicy{
			{Field: "message", Variables: true, InlineOnly: true},
		},
	})

	runner := New(registry, nopObserver{})
	results, err := runner.Run(context.Background(), &model.Project{
		Variables: []model.Entry{{Key: "user", Value: "alice"}},
	}, &model.Workflow{
		Name: "test",
		Steps: []model.Step{{
			Name:    "step",
			Type:    "fake",
			Enabled: true,
			Config: map[string]any{
				"message": "$user.id",
			},
		}},
	})
	if err != nil {
		t.Fatalf("expected successful run, got %v", err)
	}

	output, ok := results[0].Output.(map[string]any)
	if !ok {
		t.Fatalf("expected map output, got %#v", results[0].Output)
	}
	if got := output["message"]; got != "alice.id" {
		t.Fatalf("expected dot to remain literal after variable substitution, got %v", got)
	}
}

func TestRunAppliesResponseToVariables(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&fakeBlock{
		typ:             "fake",
		responseMapping: true,
		referencePolicy: []block.ReferencePolicy{
			{Field: "value", Variables: true, InlineOnly: true},
		},
		execute: func(_ context.Context, _ *block.RunContext, step model.Step) (*block.Result, error) {
			if step.Name == "source" {
				return &block.Result{
					Output: map[string]any{
						"user": map[string]any{
							"id":   "42",
							"role": "admin",
						},
						"meta": map[string]any{
							"active": true,
						},
					},
				}, nil
			}

			return &block.Result{Output: step.Config}, nil
		},
	})

	runner := New(registry, nopObserver{})
	results, err := runner.Run(context.Background(), nil, &model.Workflow{
		Name: "test",
		Steps: []model.Step{
			{
				Name:    "source",
				Type:    "fake",
				Enabled: true,
				Config: map[string]any{
					"assign": map[string]any{
						"$user_id":   "user.id",
						"$is_active": "meta.active",
					},
				},
			},
			{
				Name:    "consumer",
				Type:    "fake",
				Enabled: true,
				Config: map[string]any{
					"value": "$user_id",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("expected successful run, got %v", err)
	}

	output, ok := results[1].Output.(map[string]any)
	if !ok {
		t.Fatalf("expected map output, got %#v", results[1].Output)
	}
	if got := output["value"]; got != "42" {
		t.Fatalf("expected mapped variable to be available in next step, got %v", got)
	}
}

func TestRunAssignReadsOutputAndExportsSources(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&fakeBlock{
		typ:             "fake",
		responseMapping: true,
		referencePolicy: []block.ReferencePolicy{
			{Field: "body", Variables: true, InlineOnly: true},
			{Field: "status", Variables: true, InlineOnly: true},
		},
		execute: func(_ context.Context, _ *block.RunContext, step model.Step) (*block.Result, error) {
			if step.Name == "source" {
				return &block.Result{
					Output: map[string]any{
						"body": map[string]any{
							"id": "42",
						},
					},
					Exports: map[string]string{
						"statusCode": "201",
					},
				}, nil
			}

			return &block.Result{Output: step.Config}, nil
		},
	})

	runner := New(registry, nopObserver{})
	results, err := runner.Run(context.Background(), nil, &model.Workflow{
		Name: "test",
		Steps: []model.Step{
			{
				Name:    "source",
				Type:    "fake",
				Enabled: true,
				Config: map[string]any{
					"assign": map[string]any{
						"$user_id": "body.id",
						"$status":  "outputs.statusCode",
					},
				},
			},
			{
				Name:    "consumer",
				Type:    "fake",
				Enabled: true,
				Config: map[string]any{
					"body":   "$user_id",
					"status": "$status",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("expected successful run, got %v", err)
	}

	output, ok := results[1].Output.(map[string]any)
	if !ok {
		t.Fatalf("expected map output, got %#v", results[1].Output)
	}
	if got := output["body"]; got != "42" {
		t.Fatalf("expected assign to read output path, got %v", got)
	}
	if got := output["status"]; got != "201" {
		t.Fatalf("expected assign to read export key, got %v", got)
	}
}

func TestRunSkipsStepWhenConditionDoesNotMatch(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&fakeBlock{typ: "fake"})

	runner := New(registry, nopObserver{})
	results, err := runner.Run(context.Background(), &model.Project{
		Variables: []model.Entry{
			{Key: "status", Value: "failed"},
			{Key: "suffix", Value: "alice"},
			{Key: "cached_alice", Value: "ok"},
		},
	}, &model.Workflow{
		Name: "test",
		Steps: []model.Step{
			{
				Name:    "conditional",
				Type:    "fake",
				Enabled: true,
				Condition: &model.Condition{
					Variable: "$status",
					Operator: "=",
					Expected: "ok",
				},
				Config: map[string]any{"value": "should-not-run"},
			},
			{
				Name:    "composed",
				Type:    "fake",
				Enabled: true,
				Condition: &model.Condition{
					Variable: "$cached_$suffix",
					Operator: "=",
					Expected: "ok",
				},
				Config: map[string]any{"value": "ran"},
			},
		},
	})
	if err != nil {
		t.Fatalf("expected successful run, got %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Status != "skipped" {
		t.Fatalf("first status = %q, want skipped", results[0].Status)
	}
	if results[1].Status != "ok" {
		t.Fatalf("second status = %q, want ok", results[1].Status)
	}

	output, ok := results[0].Output.(map[string]any)
	if !ok {
		t.Fatalf("expected skipped output map, got %#v", results[0].Output)
	}
	condition, ok := output["condition"].(map[string]any)
	if !ok {
		t.Fatalf("expected condition output, got %#v", output["condition"])
	}
	if condition["actual"] != "failed" {
		t.Fatalf("condition actual = %v, want failed", condition["actual"])
	}
}

func TestRunReportsResolvedRequestAndResponseToObserver(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&fakeBlock{
		typ: "fake",
		referencePolicy: []block.ReferencePolicy{
			{Field: "value", Variables: true, InlineOnly: true},
		},
		execute: func(_ context.Context, _ *block.RunContext, step model.Step) (*block.Result, error) {
			return &block.Result{Output: map[string]any{"echo": step.Config["value"]}}, nil
		},
	})

	observer := &captureObserver{}
	runner := New(registry, observer)
	_, err := runner.Run(context.Background(), &model.Project{
		Variables: []model.Entry{{Key: "name", Value: "resolved"}},
	}, &model.Workflow{
		Name: "test",
		Steps: []model.Step{{
			Name:    "step",
			Type:    "fake",
			Enabled: true,
			Config: map[string]any{
				"value": "$name",
			},
		}},
	})
	if err != nil {
		t.Fatalf("expected successful run, got %v", err)
	}
	if len(observer.calls) != 1 {
		t.Fatalf("expected one observer call, got %d", len(observer.calls))
	}
	if observer.calls[0].start == nil || observer.calls[0].finish == nil {
		t.Fatalf("expected start and finish events, got %#v", observer.calls[0])
	}
	if got := observer.calls[0].finish.Request["value"]; got != "resolved" {
		t.Fatalf("expected resolved request value, got %v", got)
	}
	response, ok := observer.calls[0].finish.Response.(map[string]any)
	if !ok {
		t.Fatalf("expected map response, got %#v", observer.calls[0].finish.Response)
	}
	if got := response["echo"]; got != "resolved" {
		t.Fatalf("expected echoed resolved value, got %v", got)
	}
	if observer.calls[0].finish.Duration <= 0 {
		t.Fatalf("expected positive duration, got %v", observer.calls[0].finish.Duration)
	}
}

func TestRunRedactsSecretsInResultsAndObserver(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&fakeBlock{
		typ: "fake",
		referencePolicy: []block.ReferencePolicy{
			{Field: "authorization", Constants: true, InlineOnly: true},
		},
		execute: func(_ context.Context, _ *block.RunContext, _ model.Step) (*block.Result, error) {
			return nil, errors.New("leaked secret-token")
		},
	})

	observer := &captureObserver{}
	runner := New(registry, observer)
	results, err := runner.Run(context.Background(), &model.Project{
		Secrets: []model.Entry{{Key: "token", Value: "secret-token"}},
	}, &model.Workflow{
		Name: "test",
		Steps: []model.Step{{
			Name:    "step",
			Type:    "fake",
			Enabled: true,
			Config: map[string]any{
				"authorization": "Bearer @token",
			},
		}},
	})
	if err == nil || err.Error() != "leaked secret-token" {
		t.Fatalf("expected raw execution error, got %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one result, got %d", len(results))
	}
	if got := results[0].Error; got != "leaked [REDACTED]" {
		t.Fatalf("result error = %q", got)
	}
	if len(observer.calls) != 1 || observer.calls[0].finish == nil {
		t.Fatalf("expected finish observer event, got %#v", observer.calls)
	}
	if got := observer.calls[0].finish.Request["authorization"]; got != "Bearer [REDACTED]" {
		t.Fatalf("request authorization = %v", got)
	}
	if got := observer.calls[0].finish.Error.Error(); got != "leaked [REDACTED]" {
		t.Fatalf("observer error = %q", got)
	}
}

func TestRunRetryExecutesNestedBlockWithResolvedBuiltins(t *testing.T) {
	registry := NewRegistry()
	registry.Register(retryblock.New())
	attempts := 0
	registry.Register(&fakeBlock{
		typ: "fake",
		referencePolicy: []block.ReferencePolicy{
			{Field: "value", Variables: true, InlineOnly: true},
		},
		execute: func(_ context.Context, _ *block.RunContext, step model.Step) (*block.Result, error) {
			attempts++
			if attempts < 3 {
				return &block.Result{Output: map[string]any{"attempt": step.Config["value"]}}, errors.New("not ready")
			}
			return &block.Result{Output: map[string]any{"attempt": step.Config["value"]}}, nil
		},
	})

	runner := New(registry, nopObserver{})
	results, err := runner.Run(context.Background(), nil, &model.Workflow{
		Name: "test",
		Steps: []model.Step{{
			Name:    "wait",
			Type:    "retry",
			Enabled: true,
			Config: map[string]any{
				"max_attempts": 3,
				"delay_ms":     0,
				"block": map[string]any{
					"type":  "fake",
					"value": "!Retry.Attempt",
				},
			},
		}},
	})
	if err != nil {
		t.Fatalf("expected successful run, got %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
	output := results[0].Output.(map[string]any)
	if got := output["attempts"]; got != 3 {
		t.Fatalf("attempts = %v, want 3", got)
	}
}

func TestRunForeachResolvesItemBuiltinsInsideNestedBlock(t *testing.T) {
	registry := NewRegistry()
	registry.Register(foreachblock.New())
	registry.Register(&fakeBlock{
		typ: "fake",
		referencePolicy: []block.ReferencePolicy{
			{Field: "url", Variables: true, InlineOnly: true},
		},
		execute: func(_ context.Context, _ *block.RunContext, step model.Step) (*block.Result, error) {
			return &block.Result{Output: map[string]any{"url": step.Config["url"]}}, nil
		},
	})

	runner := New(registry, nopObserver{})
	results, err := runner.Run(context.Background(), &model.Project{
		Variables: []model.Entry{{Key: "users", Value: `[{"id":"42"},{"id":"99"}]`}},
	}, &model.Workflow{
		Name: "test",
		Steps: []model.Step{{
			Name:    "loop",
			Type:    "foreach",
			Enabled: true,
			Config: map[string]any{
				"items": "$users",
				"block": map[string]any{
					"type": "fake",
					"url":  "https://example.com/users/!Each.Item.id",
				},
			},
		}},
	})
	if err != nil {
		t.Fatalf("expected successful run, got %v", err)
	}

	output := results[0].Output.(map[string]any)
	loopResults := output["results"].([]map[string]any)
	first := loopResults[0]["output"].(map[string]any)
	second := loopResults[1]["output"].(map[string]any)
	if got := first["url"]; got != "https://example.com/users/42" {
		t.Fatalf("first url = %v, want users/42", got)
	}
	if got := second["url"]; got != "https://example.com/users/99" {
		t.Fatalf("second url = %v, want users/99", got)
	}
}

func TestRunForeachCollectAssignsAggregatedOutput(t *testing.T) {
	registry := NewRegistry()
	registry.Register(foreachblock.New())
	registry.Register(&fakeBlock{
		typ: "fake",
		referencePolicy: []block.ReferencePolicy{
			{Field: "value", Variables: true, InlineOnly: true},
		},
		execute: func(_ context.Context, runCtx *block.RunContext, step model.Step) (*block.Result, error) {
			if step.Name == "consumer" {
				return &block.Result{Output: step.Config}, nil
			}
			return &block.Result{
				Output: map[string]any{
					"body": map[string]any{
						"id": runCtx.Builtins["Each.Item.id"],
					},
				},
			}, nil
		},
	})

	runner := New(registry, nopObserver{})
	results, err := runner.Run(context.Background(), &model.Project{
		Variables: []model.Entry{{Key: "users", Value: `[{"id":"42"},{"id":"99"}]`}},
	}, &model.Workflow{
		Name: "test",
		Steps: []model.Step{
			{
				Name:    "loop",
				Type:    "foreach",
				Enabled: true,
				Config: map[string]any{
					"items": "$users",
					"block": map[string]any{
						"type": "fake",
					},
					"collect": map[string]any{
						"$user_ids_json": "output.body.id",
					},
				},
			},
			{
				Name:    "consumer",
				Type:    "fake",
				Enabled: true,
				Config: map[string]any{
					"value": "$user_ids_json",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("expected successful run, got %v", err)
	}

	output := results[1].Output.(map[string]any)
	if got := output["value"]; got != `["42","99"]` {
		t.Fatalf("value = %v, want JSON array", got)
	}
}

func TestRunTransformBuildsJSONForNextStep(t *testing.T) {
	registry := NewRegistry()
	registry.Register(transformblock.New())
	registry.Register(&fakeBlock{
		typ: "fake",
		referencePolicy: []block.ReferencePolicy{
			{Field: "body", Variables: true, InlineOnly: true},
		},
		execute: func(_ context.Context, _ *block.RunContext, step model.Step) (*block.Result, error) {
			return &block.Result{Output: step.Config}, nil
		},
	})

	runner := New(registry, nopObserver{})
	results, err := runner.Run(context.Background(), &model.Project{
		Variables: []model.Entry{
			{Key: "user_id", Value: "42"},
			{Key: "active", Value: "true"},
			{Key: "name", Value: "Alice"},
		},
	}, &model.Workflow{
		Name: "test",
		Steps: []model.Step{
			{
				Name:    "transform",
				Type:    "transform",
				Enabled: true,
				Config: map[string]any{
					"value": map[string]any{
						"user": map[string]any{
							"id":     "$user_id",
							"name":   "$name",
							"active": "$active",
						},
					},
					"casts": map[string]any{
						"user.id":     "int",
						"user.active": "bool",
					},
					"assign": map[string]any{
						"$payload_json": "json",
					},
				},
			},
			{
				Name:    "consumer",
				Type:    "fake",
				Enabled: true,
				Config: map[string]any{
					"body": "$payload_json",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("expected successful run, got %v", err)
	}

	output := results[1].Output.(map[string]any)
	if got := output["body"]; got != `{"user":{"active":true,"id":42,"name":"Alice"}}` {
		t.Fatalf("body = %v", got)
	}
}

func TestRunWritesSecretVariablesViaSetVarsAndRedactsThem(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&fakeBlock{
		typ: "fake",
		referencePolicy: []block.ReferencePolicy{
			{Field: "authorization", Variables: true, InlineOnly: true},
		},
	})
	registry.Register(workflowblock.New())
	registry.Register(&fakeBlock{
		typ: "writer",
		execute: func(_ context.Context, _ *block.RunContext, _ model.Step) (*block.Result, error) {
			return &block.Result{
				Output:  map[string]any{"token": "runtime-secret"},
				Exports: map[string]string{"child_token": "runtime-secret"},
			}, nil
		},
	})

	runner := New(registry, nopObserver{})
	workflows := map[string]*model.Workflow{
		"parent": {
			ID:              workflowID("parent"),
			Name:            "parent",
			SecretVariables: []model.Entry{{Key: "child_token", Value: ""}},
			Steps: []model.Step{
				{
					Name:    "nested",
					Type:    "workflow",
					Enabled: true,
					Config: map[string]any{
						"target_workflow_id": workflowID("child"),
					},
				},
				{
					Name:    "consumer",
					Type:    "fake",
					Enabled: true,
					Config: map[string]any{
						"authorization": "Bearer $child_token",
					},
				},
			},
		},
		"child": {
			ID:              workflowID("child"),
			Name:            "child",
			Outputs:         map[string]string{"child_token": "$child_token"},
			SecretVariables: []model.Entry{{Key: "child_token", Value: ""}},
			Steps: []model.Step{
				{
					Name:    "child-step",
					Type:    "writer",
					Enabled: true,
				},
			},
		},
	}

	results, err := runner.RunWithWorkflows(context.Background(), nil, workflows, workflows["parent"])
	if err != nil {
		t.Fatalf("expected successful run, got %v", err)
	}

	nestedOutput := results[0].Output.(map[string]any)
	secretVariables := nestedOutput["secret_variables"].(map[string]string)
	if got := secretVariables["child_token"]; got != "[REDACTED]" {
		t.Fatalf("nested secret variable = %q", got)
	}
	outputs := nestedOutput["outputs"].(map[string]string)
	if got := outputs["child_token"]; got != "[REDACTED]" {
		t.Fatalf("nested export = %q", got)
	}
	consumerOutput := results[1].Output.(map[string]any)
	if got := consumerOutput["authorization"]; got != "Bearer [REDACTED]" {
		t.Fatalf("consumer authorization = %v", got)
	}
}

func TestRunRejectsWritesToReadOnlyAtTargets(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&fakeBlock{
		typ:             "fake",
		responseMapping: true,
		execute: func(_ context.Context, _ *block.RunContext, _ model.Step) (*block.Result, error) {
			return &block.Result{
				Output:  map[string]any{"token": "rotated"},
				Exports: map[string]string{"token": "rotated"},
			}, nil
		},
	})

	runner := New(registry, nopObserver{})
	results, err := runner.Run(context.Background(), &model.Project{
		Secrets: []model.Entry{{Key: "token", Value: "secret-token"}},
	}, &model.Workflow{
		Name: "test",
		Steps: []model.Step{{
			Name:    "step",
			Type:    "fake",
			Enabled: true,
			Config: map[string]any{
				"assign": map[string]any{"$token": "body.token"},
			},
		}},
	})
	if err == nil || err.Error() != "cannot write to read-only secret @token" {
		t.Fatalf("expected read-only secret error, got %v", err)
	}
	if len(results) != 1 || results[0].Error != "cannot write to read-only secret @token" {
		t.Fatalf("unexpected results %#v", results)
	}
}

func TestRunCanReadErrorValuesFromReadOnlyNamespaceInCatchErrorBlocks(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&fakeBlock{
		typ:        "boom",
		executeErr: errors.New("boom"),
	})
	registry.Register(&fakeBlock{
		typ: "reader",
		referencePolicy: []block.ReferencePolicy{
			{Field: "message", Constants: true, InlineOnly: true},
		},
	})

	runner := New(registry, nopObserver{})
	results, err := runner.Run(context.Background(), nil, &model.Workflow{
		Name: "test",
		Steps: []model.Step{{
			Name:    "step",
			Type:    "boom",
			Enabled: true,
		}},
		OnErrorSteps: []model.Step{{
			Name:    "handler",
			Type:    "reader",
			Enabled: true,
			Config: map[string]any{
				"message": "!ErrorMessage",
			},
		}},
	})
	if err == nil || err.Error() != "boom" {
		t.Fatalf("expected original failure, got %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected failed step and successful handler, got %#v", results)
	}
	if results[1].Status != "error-handler-ok" {
		t.Fatalf("handler status = %q", results[1].Status)
	}
	output, ok := results[1].Output.(map[string]any)
	if !ok {
		t.Fatalf("handler output = %#v", results[1].Output)
	}
	if got := output["message"]; got != "boom" {
		t.Fatalf("handler message = %v", got)
	}
}

func TestRunContinuesAfterIgnoredStepFailure(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&fakeBlock{
		typ:        "boom",
		executeErr: errors.New("telegram unavailable"),
	})
	registry.Register(&fakeBlock{typ: "fake"})

	runner := New(registry, nopObserver{})
	results, err := runner.Run(context.Background(), nil, &model.Workflow{
		Name: "test",
		Steps: []model.Step{
			{
				Name:            "notify",
				Type:            "boom",
				Enabled:         true,
				ContinueOnError: true,
			},
			{
				Name:    "after",
				Type:    "fake",
				Enabled: true,
				Config:  map[string]any{"value": "ran"},
			},
		},
		OnErrorSteps: []model.Step{{
			Name:    "handler",
			Type:    "fake",
			Enabled: true,
			Config:  map[string]any{"value": "should-not-run"},
		}},
	})
	if err != nil {
		t.Fatalf("expected ignored failure to continue workflow, got %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 step results, got %#v", results)
	}
	if results[0].Status != stepStatusIgnoredError {
		t.Fatalf("first status = %q, want %q", results[0].Status, stepStatusIgnoredError)
	}
	if results[0].Error != "telegram unavailable" {
		t.Fatalf("first error = %q, want %q", results[0].Error, "telegram unavailable")
	}
	if results[1].Status != stepStatusOK {
		t.Fatalf("second status = %q, want %q", results[1].Status, stepStatusOK)
	}
}

func TestRunExposesErrorBlockIDToCatchErrorBlocks(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&fakeBlock{
		typ:        "boom",
		executeErr: errors.New("boom"),
	})
	registry.Register(&fakeBlock{
		typ: "reader",
		referencePolicy: []block.ReferencePolicy{
			{Field: "message", Constants: true, InlineOnly: true},
		},
	})

	runner := New(registry, nopObserver{})
	results, err := runner.Run(context.Background(), nil, &model.Workflow{
		Name: "test",
		Steps: []model.Step{{
			BlockID: "seed_postgres",
			Name:    "Seed Postgres",
			Type:    "boom",
			Enabled: true,
		}},
		OnErrorSteps: []model.Step{{
			Name:    "handler",
			Type:    "reader",
			Enabled: true,
			Config: map[string]any{
				"message": "!ErrorBlockID",
			},
		}},
	})
	if err == nil || err.Error() != "boom" {
		t.Fatalf("expected original failure, got %v", err)
	}

	output, ok := results[1].Output.(map[string]any)
	if !ok {
		t.Fatalf("handler output = %#v", results[1].Output)
	}
	if got := output["message"]; got != "seed_postgres" {
		t.Fatalf("handler message = %v", got)
	}
}

func TestRunWithWorkflowsExposesParentBuiltinsToChildWorkflow(t *testing.T) {
	registry := NewRegistry()
	registry.Register(workflowblock.New())
	registry.Register(&fakeBlock{
		typ: "fake",
		referencePolicy: []block.ReferencePolicy{
			{Field: "value", Constants: true, Variables: true, InlineOnly: true},
		},
		execute: func(_ context.Context, _ *block.RunContext, step model.Step) (*block.Result, error) {
			return &block.Result{Output: step.Config}, nil
		},
	})

	runner := New(registry, nopObserver{})
	workflows := map[string]*model.Workflow{
		"parent": {
			ID:   workflowID("parent"),
			Name: "parent",
			Steps: []model.Step{
				{
					Name:    "nested",
					Type:    "workflow",
					Enabled: true,
					Config: map[string]any{
						"target_workflow_id": workflowID("child"),
					},
				},
			},
		},
		"child": {
			ID:   workflowID("child"),
			Name: "child",
			Steps: []model.Step{
				{
					Name:    "reader",
					Type:    "fake",
					Enabled: true,
					Config: map[string]any{
						"value": "!Parent.WorkflowID",
					},
				},
			},
		},
	}

	results, err := runner.RunWithWorkflows(context.Background(), nil, workflows, workflows["parent"])
	if err != nil {
		t.Fatalf("expected successful run, got %v", err)
	}

	nestedOutput := results[0].Output.(map[string]any)
	steps := nestedOutput["steps"].([]block.WorkflowStepResult)
	readerOutput, ok := steps[0].Output.(map[string]any)
	if !ok {
		t.Fatalf("expected child step output map, got %#v", steps[0].Output)
	}
	if got := readerOutput["value"]; got != workflowID("parent") {
		t.Fatalf("child builtin value = %v, want %q", got, workflowID("parent"))
	}
}

func TestRunWithWorkflowsRedactsNestedWorkflowOutputButKeepsRawExports(t *testing.T) {
	registry := NewRegistry()
	registry.Register(workflowblock.New())
	registry.Register(&fakeBlock{
		typ: "fake",
		referencePolicy: []block.ReferencePolicy{
			{Field: "child_token", Variables: true, InlineOnly: true},
		},
		execute: func(_ context.Context, _ *block.RunContext, step model.Step) (*block.Result, error) {
			if step.Name == "child-step" {
				return &block.Result{
					Output:  map[string]any{"token": "secret-token"},
					Exports: map[string]string{"child_token": "secret-token"},
				}, nil
			}
			return &block.Result{
				Output: map[string]any{
					"raw_seen": step.Config["child_token"] == "secret-token",
				},
			}, nil
		},
	})

	runner := New(registry, nopObserver{})
	workflows := map[string]*model.Workflow{
		"parent": {
			ID:   workflowID("parent"),
			Name: "parent",
			Steps: []model.Step{
				{
					Name:    "nested",
					Type:    "workflow",
					Enabled: true,
					Config: map[string]any{
						"target_workflow_id": workflowID("child"),
					},
				},
				{
					Name:    "consumer",
					Type:    "fake",
					Enabled: true,
					Config: map[string]any{
						"child_token": "$child_token",
					},
				},
			},
		},
		"child": {
			ID:      workflowID("child"),
			Name:    "child",
			Outputs: map[string]string{"child_token": "$child_token"},
			Steps: []model.Step{
				{
					Name:    "child-step",
					Type:    "fake",
					Enabled: true,
				},
			},
		},
	}

	results, err := runner.RunWithWorkflows(context.Background(), &model.Project{
		Secrets: []model.Entry{{Key: "token", Value: "secret-token"}},
	}, workflows, workflows["parent"])
	if err != nil {
		t.Fatalf("expected successful run, got %v", err)
	}

	nestedOutput := results[0].Output.(map[string]any)
	outputs := nestedOutput["outputs"].(map[string]string)
	if got := outputs["child_token"]; got != "[REDACTED]" {
		t.Fatalf("nested output export = %q", got)
	}
	variables := nestedOutput["variables"].(map[string]string)
	if got := variables["child_token"]; got != "[REDACTED]" {
		t.Fatalf("nested output variable = %q", got)
	}
	consumerOutput := results[1].Output.(map[string]any)
	if got := consumerOutput["raw_seen"]; got != true {
		t.Fatalf("raw export should remain available for execution, got %#v", consumerOutput)
	}
}

func TestRunWithWorkflowsWithoutDeclaredOutputsExportsNothing(t *testing.T) {
	registry := NewRegistry()
	registry.Register(workflowblock.New())
	registry.Register(&fakeBlock{
		typ: "fake",
		referencePolicy: []block.ReferencePolicy{
			{Field: "value", Variables: true, InlineOnly: true},
		},
		execute: func(_ context.Context, _ *block.RunContext, step model.Step) (*block.Result, error) {
			switch step.Name {
			case "child-step":
				value := step.Config["value"].(string)
				return &block.Result{
					Output: map[string]any{"echo": value},
					Exports: map[string]string{
						"child_output": value,
					},
				}, nil
			default:
				return &block.Result{Output: step.Config}, nil
			}
		},
	})

	runner := New(registry, nopObserver{})
	workflows := map[string]*model.Workflow{
		"parent": {
			ID:   workflowID("parent"),
			Name: "parent",
			Steps: []model.Step{{
				Name:    "nested",
				Type:    "workflow",
				Enabled: true,
				Config: map[string]any{
					"target_workflow_id": workflowID("child"),
					"inputs": map[string]any{
						"incoming": "value-42",
					},
				},
			}},
		},
		"child": {
			ID:   workflowID("child"),
			Name: "child",
			Steps: []model.Step{
				{
					Name:    "child-step",
					Type:    "fake",
					Enabled: true,
					Config: map[string]any{
						"value": "$incoming",
					},
				},
			},
		},
	}

	results, err := runner.RunWithWorkflows(context.Background(), nil, workflows, workflows["parent"])
	if err != nil {
		t.Fatalf("expected successful run, got %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	nestedOutput, ok := results[0].Output.(map[string]any)
	if !ok {
		t.Fatalf("expected nested output map, got %#v", results[0].Output)
	}
	if got := nestedOutput["workflow_id"]; got != workflowID("child") {
		t.Fatalf("workflow_id = %v, want %q", got, workflowID("child"))
	}
	outputs, ok := nestedOutput["outputs"].(map[string]string)
	if !ok {
		t.Fatalf("expected outputs map, got %#v", nestedOutput["outputs"])
	}
	if len(outputs) != 0 {
		t.Fatalf("expected no exported outputs without workflow.outputs, got %#v", outputs)
	}
}

func TestRunWithWorkflowsUsesDeclaredWorkflowOutputsWhenPresent(t *testing.T) {
	registry := NewRegistry()
	registry.Register(workflowblock.New())
	registry.Register(&fakeBlock{
		typ: "fake",
		referencePolicy: []block.ReferencePolicy{
			{Field: "value", Variables: true, InlineOnly: true},
		},
		execute: func(_ context.Context, _ *block.RunContext, step model.Step) (*block.Result, error) {
			switch step.Name {
			case "child-step":
				value := step.Config["value"].(string)
				return &block.Result{
					Output:  map[string]any{"echo": value},
					Exports: map[string]string{"child_output": value},
				}, nil
			default:
				return &block.Result{Output: step.Config}, nil
			}
		},
	})

	runner := New(registry, nopObserver{})
	workflows := map[string]*model.Workflow{
		"parent": {
			ID:   workflowID("parent"),
			Name: "parent",
			Steps: []model.Step{{
				Name:    "nested",
				Type:    "workflow",
				Enabled: true,
				Config: map[string]any{
					"target_workflow_id": workflowID("child"),
					"inputs": map[string]any{
						"incoming": "value-42",
					},
				},
			}},
		},
		"child": {
			ID:      workflowID("child"),
			Name:    "child",
			Outputs: map[string]string{"public_value": "$child_output"},
			Steps: []model.Step{
				{
					Name:    "child-step",
					Type:    "fake",
					Enabled: true,
					Config: map[string]any{
						"value": "$incoming",
					},
				},
			},
		},
	}

	results, err := runner.RunWithWorkflows(context.Background(), nil, workflows, workflows["parent"])
	if err != nil {
		t.Fatalf("expected successful run, got %v", err)
	}
	nestedOutput := results[0].Output.(map[string]any)
	outputs := nestedOutput["outputs"].(map[string]string)
	if len(outputs) != 1 {
		t.Fatalf("outputs = %#v", outputs)
	}
	if got := outputs["public_value"]; got != "value-42" {
		t.Fatalf("public_value = %q, want %q", got, "value-42")
	}
	if _, ok := outputs["ignored"]; ok {
		t.Fatalf("unexpected implicit output in %#v", outputs)
	}
}

func TestRunRejectsNestedWorkflowSteps(t *testing.T) {
	registry := NewRegistry()
	registry.Register(workflowblock.New())

	runner := New(registry, nopObserver{})
	_, err := runner.Run(context.Background(), nil, &model.Workflow{
		Name: "parent",
		Steps: []model.Step{{
			Name: "nested",
			Type: "workflow",
			Config: map[string]any{
				"target_workflow_id": workflowID("child"),
			},
		}},
	})
	if err == nil {
		t.Fatal("Run() error = nil, want error")
	}
	if err.Error() != "nested workflow steps require RunWithWorkflows" {
		t.Fatalf("Run() error = %q", err.Error())
	}
}

func TestRunWithWorkflowsReturnsHelpfulErrorForUnknownWorkflowID(t *testing.T) {
	registry := NewRegistry()
	registry.Register(workflowblock.New())

	runner := New(registry, nopObserver{})
	workflows := map[string]*model.Workflow{
		"parent": {
			ID:   workflowID("parent"),
			Name: "parent",
			Steps: []model.Step{{
				Name:    "nested",
				Type:    "workflow",
				Enabled: true,
				Config: map[string]any{
					"target_workflow_id": "missing-child",
				},
			}},
		},
		"child": {
			ID:   workflowID("child"),
			Name: "child",
		},
	}

	_, err := runner.RunWithWorkflows(context.Background(), nil, workflows, workflows["parent"])
	if err == nil {
		t.Fatal("RunWithWorkflows() error = nil, want error")
	}
	if got := err.Error(); !strings.Contains(got, `available workflow ids`) {
		t.Fatalf("error = %q", got)
	}
	if got := err.Error(); !strings.Contains(got, "child -> "+workflowID("child")) {
		t.Fatalf("error = %q", got)
	}
}

func TestRunWithWorkflowsTimesOutWholeWorkflow(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&fakeBlock{
		typ: "wait",
		execute: func(ctx context.Context, _ *block.RunContext, _ model.Step) (*block.Result, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	})

	runner := New(registry, nopObserver{})
	results, err := runner.Run(context.Background(), nil, &model.Workflow{
		Name:           "timeout test",
		TimeoutSeconds: 1,
		Steps: []model.Step{{
			Name:    "wait forever",
			Type:    "wait",
			Enabled: true,
		}},
	})
	if err == nil {
		t.Fatal("Run() error = nil, want timeout")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context deadline exceeded", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one failed result, got %#v", results)
	}
	if results[0].Status != stepStatusFailed {
		t.Fatalf("status = %q, want %q", results[0].Status, stepStatusFailed)
	}
	if !strings.Contains(results[0].Error, context.DeadlineExceeded.Error()) {
		t.Fatalf("step error = %q", results[0].Error)
	}
}

func workflowID(name string) string {
	return name
}
