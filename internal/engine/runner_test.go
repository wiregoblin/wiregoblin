package engine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/models"
	workflowblock "github.com/wiregoblin/wiregoblin/internal/steps/workflow"
)

type observerCall struct {
	start  *StepStartEvent
	finish *StepFinishEvent
}

type captureObserver struct {
	calls []observerCall
}

func (o *captureObserver) OnStepStart(event StepStartEvent) {
	o.calls = append(o.calls, observerCall{start: &event})
}

func (o *captureObserver) OnStepFinish(event StepFinishEvent) {
	for i := range o.calls {
		if o.calls[i].start != nil && o.calls[i].finish == nil {
			o.calls[i].finish = &event
			return
		}
	}
	o.calls = append(o.calls, observerCall{finish: &event})
}

type fakeBlock struct {
	typ             string
	validateErr     error
	executeErr      error
	output          any
	execute         func(ctx context.Context, runCtx *block.RunContext, step models.Step) (*block.Result, error)
	referencePolicy []block.ReferencePolicy
	responseMapping bool
}

func (b *fakeBlock) Type() string {
	return b.typ
}

func (b *fakeBlock) Validate(_ models.Step) error {
	return b.validateErr
}

func (b *fakeBlock) Execute(ctx context.Context, runCtx *block.RunContext, step models.Step) (*block.Result, error) {
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

	runner := New(registry)
	results, err := runner.Run(context.Background(), nil, &models.Workflow{
		Name: "test",
		Steps: []models.Step{{
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

func TestRunRejectsUnsupportedResponseMapping(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&fakeBlock{
		typ:             "fake",
		responseMapping: false,
	})

	runner := New(registry)
	_, err := runner.Run(context.Background(), nil, &models.Workflow{
		Name: "test",
		Steps: []models.Step{{
			Name:    "step",
			Type:    "fake",
			Enabled: true,
			Config: map[string]any{
				"response_mapping": []any{map[string]any{"key": "value", "path": "body.id"}},
			},
		}},
	})
	if err == nil || err.Error() != `block type "fake" does not support response mapping` {
		t.Fatalf("expected unsupported response mapping error, got %v", err)
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

	runner := New(registry)
	results, err := runner.Run(context.Background(), &models.Project{
		Variables: []models.Entry{{Key: "name", Value: "resolved"}},
	}, &models.Workflow{
		Name: "test",
		Steps: []models.Step{{
			Name:    "step",
			Type:    "fake",
			Enabled: true,
			Config: map[string]any{
				"allowed": "@name",
				"denied":  "@name",
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
	if got := output["denied"]; got != "@name" {
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

	runner := New(registry)
	results, err := runner.Run(context.Background(), &models.Project{
		Constants: []models.Entry{{Key: "users_url", Value: "https://example.com/users/1"}},
	}, &models.Workflow{
		Name: "test",
		Steps: []models.Step{{
			Name:    "step",
			Type:    "fake",
			Enabled: true,
			Config: map[string]any{
				"url": "$users_url",
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

	runner := New(registry)
	results, err := runner.Run(context.Background(), &models.Project{
		Constants: []models.Entry{{Key: "scheme", Value: "Bearer"}},
		Variables: []models.Entry{{Key: "token", Value: "secret-token"}},
	}, &models.Workflow{
		Name: "test",
		Steps: []models.Step{{
			Name:    "step",
			Type:    "fake",
			Enabled: true,
			Config: map[string]any{
				"authorization": "$scheme @token",
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

func TestRunDoesNotTreatDotAsPartOfReferenceName(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&fakeBlock{
		typ: "fake",
		referencePolicy: []block.ReferencePolicy{
			{Field: "message", Variables: true, InlineOnly: true},
		},
	})

	runner := New(registry)
	results, err := runner.Run(context.Background(), &models.Project{
		Variables: []models.Entry{{Key: "user", Value: "alice"}},
	}, &models.Workflow{
		Name: "test",
		Steps: []models.Step{{
			Name:    "step",
			Type:    "fake",
			Enabled: true,
			Config: map[string]any{
				"message": "@user.id",
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

func TestRunAppliesResponseMappingToVariables(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&fakeBlock{
		typ:             "fake",
		responseMapping: true,
		referencePolicy: []block.ReferencePolicy{
			{Field: "value", Variables: true, InlineOnly: true},
		},
		execute: func(_ context.Context, _ *block.RunContext, step models.Step) (*block.Result, error) {
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

	runner := New(registry)
	results, err := runner.Run(context.Background(), nil, &models.Workflow{
		Name: "test",
		Steps: []models.Step{
			{
				Name:    "source",
				Type:    "fake",
				Enabled: true,
				Config: map[string]any{
					"response_mapping": []any{
						map[string]any{"key": "user_id", "path": "user.id"},
						map[string]any{"key": "is_active", "path": "meta.active"},
					},
				},
			},
			{
				Name:    "consumer",
				Type:    "fake",
				Enabled: true,
				Config: map[string]any{
					"value": "@user_id",
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

func TestRunReportsResolvedRequestAndResponseToObserver(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&fakeBlock{
		typ: "fake",
		referencePolicy: []block.ReferencePolicy{
			{Field: "value", Variables: true, InlineOnly: true},
		},
		execute: func(_ context.Context, _ *block.RunContext, step models.Step) (*block.Result, error) {
			return &block.Result{Output: map[string]any{"echo": step.Config["value"]}}, nil
		},
	})

	observer := &captureObserver{}
	runner := New(registry, WithObserver(observer))
	_, err := runner.Run(context.Background(), &models.Project{
		Variables: []models.Entry{{Key: "name", Value: "resolved"}},
	}, &models.Workflow{
		Name: "test",
		Steps: []models.Step{{
			Name:    "step",
			Type:    "fake",
			Enabled: true,
			Config: map[string]any{
				"value": "@name",
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
			{Field: "authorization", Variables: true, InlineOnly: true},
		},
		execute: func(_ context.Context, _ *block.RunContext, _ models.Step) (*block.Result, error) {
			return nil, errors.New("leaked secret-token")
		},
	})

	observer := &captureObserver{}
	runner := New(registry, WithObserver(observer))
	results, err := runner.Run(context.Background(), &models.Project{
		Secrets: []models.Entry{{Key: "token", Value: "secret-token"}},
	}, &models.Workflow{
		Name: "test",
		Steps: []models.Step{{
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

func TestRunWithWorkflowsRedactsNestedWorkflowOutputButKeepsRawExports(t *testing.T) {
	registry := NewRegistry()
	registry.Register(workflowblock.New())
	registry.Register(&fakeBlock{
		typ: "fake",
		referencePolicy: []block.ReferencePolicy{
			{Field: "child_token", Variables: true, InlineOnly: true},
		},
		execute: func(_ context.Context, _ *block.RunContext, step models.Step) (*block.Result, error) {
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

	runner := New(registry)
	workflows := map[string]*models.Workflow{
		"parent": {
			ID:   workflowID("parent"),
			Name: "parent",
			Steps: []models.Step{
				{
					Name:    "nested",
					Type:    "workflow",
					Enabled: true,
					Config: map[string]any{
						"target_workflow_uid": workflowID("child").String(),
					},
				},
				{
					Name:    "consumer",
					Type:    "fake",
					Enabled: true,
					Config: map[string]any{
						"child_token": "@child_token",
					},
				},
			},
		},
		"child": {
			ID:   workflowID("child"),
			Name: "child",
			Steps: []models.Step{
				{
					Name:    "child-step",
					Type:    "fake",
					Enabled: true,
				},
			},
		},
	}

	results, err := runner.RunWithWorkflows(context.Background(), &models.Project{
		Secrets: []models.Entry{{Key: "token", Value: "secret-token"}},
	}, workflows, workflows["parent"])
	if err != nil {
		t.Fatalf("expected successful run, got %v", err)
	}

	nestedOutput := results[0].Output.(map[string]any)
	exports := nestedOutput["exports"].(map[string]string)
	if got := exports["child_token"]; got != "[REDACTED]" {
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

func TestRunWithWorkflowsExecutesNestedWorkflowAndExportsChangedVariables(t *testing.T) {
	registry := NewRegistry()
	registry.Register(workflowblock.New())
	registry.Register(&fakeBlock{
		typ: "fake",
		referencePolicy: []block.ReferencePolicy{
			{Field: "value", Variables: true, InlineOnly: true},
		},
		execute: func(_ context.Context, _ *block.RunContext, step models.Step) (*block.Result, error) {
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

	runner := New(registry)
	workflows := map[string]*models.Workflow{
		"parent": {
			ID:   workflowID("parent"),
			Name: "parent",
			Steps: []models.Step{
				{
					Name:    "nested",
					Type:    "workflow",
					Enabled: true,
					Config: map[string]any{
						"target_workflow_uid": workflowID("child").String(),
						"input_mapping": map[string]any{
							"incoming": "value-42",
						},
					},
				},
				{
					Name:    "consumer",
					Type:    "fake",
					Enabled: true,
					Config: map[string]any{
						"value": "@child_output",
					},
				},
			},
		},
		"child": {
			ID:   workflowID("child"),
			Name: "child",
			Steps: []models.Step{
				{
					Name:    "child-step",
					Type:    "fake",
					Enabled: true,
					Config: map[string]any{
						"value": "@incoming",
					},
				},
			},
		},
	}

	results, err := runner.RunWithWorkflows(context.Background(), nil, workflows, workflows["parent"])
	if err != nil {
		t.Fatalf("expected successful run, got %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	nestedOutput, ok := results[0].Output.(map[string]any)
	if !ok {
		t.Fatalf("expected nested output map, got %#v", results[0].Output)
	}
	if got := nestedOutput["workflow"]; got != "child" {
		t.Fatalf("workflow = %v, want %q", got, "child")
	}
	exports, ok := nestedOutput["exports"].(map[string]string)
	if !ok {
		t.Fatalf("expected exports map, got %#v", nestedOutput["exports"])
	}
	if got := exports["child_output"]; got != "value-42" {
		t.Fatalf("child_output export = %q, want %q", got, "value-42")
	}

	consumerOutput, ok := results[1].Output.(map[string]any)
	if !ok {
		t.Fatalf("expected consumer output map, got %#v", results[1].Output)
	}
	if got := consumerOutput["value"]; got != "value-42" {
		t.Fatalf("consumer value = %v, want %q", got, "value-42")
	}
}

func TestRunRejectsNestedWorkflowSteps(t *testing.T) {
	registry := NewRegistry()
	registry.Register(workflowblock.New())

	runner := New(registry)
	_, err := runner.Run(context.Background(), nil, &models.Workflow{
		Name: "parent",
		Steps: []models.Step{{
			Name: "nested",
			Type: "workflow",
			Config: map[string]any{
				"target_workflow_uid": workflowID("child").String(),
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

func TestRunWithWorkflowsReturnsHelpfulErrorForUnknownWorkflowUUID(t *testing.T) {
	registry := NewRegistry()
	registry.Register(workflowblock.New())

	runner := New(registry)
	workflows := map[string]*models.Workflow{
		"parent": {
			ID:   workflowID("parent"),
			Name: "parent",
			Steps: []models.Step{{
				Name:    "nested",
				Type:    "workflow",
				Enabled: true,
				Config: map[string]any{
					"target_workflow_uid": "child",
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
	if got := err.Error(); !strings.Contains(got, `target_workflow_uid expects a workflow UUID`) {
		t.Fatalf("error = %q", got)
	}
	if got := err.Error(); !strings.Contains(got, "child -> "+workflowID("child").String()) {
		t.Fatalf("error = %q", got)
	}
}

func workflowID(name string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(name))
}
