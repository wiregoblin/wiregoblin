package workflow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/model"
)

func TestEmitWorkflowFinishedRedactsSecrets(t *testing.T) {
	t.Parallel()

	events := make(chan model.RunEvent, 1)
	service := &Service{}

	service.emitWorkflowFinished(
		events,
		"demo-id", "Demo",
		"alpha-id", "Alpha",
		1,
		time.Now(),
		errors.New("token secret-123 leaked"),
		[]string{"secret-123"},
	)

	event := <-events
	if event.Status != "failed" {
		t.Fatalf("status = %q, want failed", event.Status)
	}
	if event.Error != "token [REDACTED] leaked" {
		t.Fatalf("error = %q", event.Error)
	}
}

func TestSecretValuesFromProjectIncludesSecretVariables(t *testing.T) {
	values := secretValuesFromProject(&model.Project{
		Secrets:         []model.Entry{{Key: "api_token", Value: "secret-123"}},
		SecretVariables: []model.Entry{{Key: "session_token", Value: "runtime-456"}},
	})

	if len(values) != 2 {
		t.Fatalf("len(values) = %d, want 2", len(values))
	}
	if values[0] != "runtime-456" && values[1] != "runtime-456" {
		t.Fatalf("runtime secret variable missing from %v", values)
	}
}

func TestRunWorkflowRejectsDirectRunWhenDisallowed(t *testing.T) {
	t.Parallel()

	service := New(stubProjectRepository{
		project: &model.Definition{
			Meta: &model.Project{ID: "demo", Name: "Demo"},
			WorkflowByID: map[string]*model.Workflow{
				"child": {
					ID:         "child",
					Name:       "Child",
					DisableRun: true,
				},
			},
		},
	}, stubBlockRegistry{})

	events, err := service.RunWorkflow(context.Background(), "demo", "child", RunOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	if events != nil {
		t.Fatal("expected nil events channel")
	}
	if got := err.Error(); got != `workflow "child" cannot be run directly; invoke it via a workflow block` {
		t.Fatalf("error = %q", got)
	}
}

type stubProjectRepository struct {
	project *model.Definition
}

func (s stubProjectRepository) GetProject(context.Context, string) (*model.Definition, error) {
	return s.project, nil
}

func (s stubProjectRepository) GetWorkflow(_ context.Context, _ string, workflowID string) (*model.Workflow, error) {
	return s.project.WorkflowByID[workflowID], nil
}

func (s stubProjectRepository) ListWorkflows(context.Context, string) ([]string, error) {
	return nil, nil
}

type stubBlockRegistry struct{}

func (stubBlockRegistry) Get(model.BlockType) (block.Block, error) { return nil, nil }

func (stubBlockRegistry) Close() {}
