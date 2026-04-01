// Package workflow coordinates workflow loading and execution streams.
package workflow

import (
	"context"
	"fmt"
	"time"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/engine"
	"github.com/wiregoblin/wiregoblin/internal/model"
	"github.com/wiregoblin/wiregoblin/internal/redact"
)

type (
	projectRepository interface {
		GetProject(ctx context.Context, projectID string) (*model.Definition, error)
		GetWorkflow(ctx context.Context, projectID string, workflowID string) (*model.Workflow, error)
		ListWorkflows(ctx context.Context, projectID string) ([]string, error)
	}

	blockRegistry interface {
		Get(blockType model.BlockType) (block.Block, error)
		Close()
	}
)

// Service coordinates workflow loading and execution.
type Service struct {
	projects projectRepository
	registry blockRegistry
}

// RunOptions configures one workflow run.
type RunOptions struct {
	Context  context.Context
	Observer engine.Observer
}

// New creates a workflow service around the provided project repository and block registry.
func New(projects projectRepository, registry blockRegistry) *Service {
	return &Service{projects: projects, registry: registry}
}

// Close releases resources held by the block registry.
func (s *Service) Close() {
	s.registry.Close()
}

// ListWorkflows returns all workflow IDs defined in the project, sorted alphabetically.
func (s *Service) ListWorkflows(ctx context.Context, projectID string) ([]string, error) {
	return s.projects.ListWorkflows(ctx, projectID)
}

// RunWorkflow validates setup and returns a stream of execution events.
func (s *Service) RunWorkflow(
	ctx context.Context,
	projectID string,
	workflowID string,
	opts RunOptions,
) (<-chan model.RunEvent, error) {
	wfDef, err := s.projects.GetWorkflow(ctx, projectID, workflowID)
	if err != nil {
		return nil, err
	}

	project, err := s.projects.GetProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("load project %s: %w", projectID, err)
	}

	events := make(chan model.RunEvent)

	go func() {
		defer close(events)
		s.executeWorkflow(project.Meta, project.Workflows, wfDef, opts, events)
	}()

	return events, nil
}

func (s *Service) executeWorkflow(
	projectMeta *model.Project,
	workflows map[string]*model.Workflow,
	wfDef *model.Workflow,
	opts RunOptions,
	events chan<- model.RunEvent,
) {
	startedAt := time.Now()
	secretValues := secretValuesFromProject(projectMeta)
	events <- model.RunEvent{
		Type:         model.EventWorkflowStarted,
		ProjectID:    projectMeta.ID,
		ProjectName:  projectMeta.Name,
		WorkflowID:   wfDef.ID,
		WorkflowName: wfDef.Name,
		Total:        len(wfDef.Steps),
		Status:       "running",
	}

	if len(wfDef.Steps) == 0 {
		s.emitWorkflowFinished(
			events,
			projectMeta.ID, projectMeta.Name,
			wfDef.ID, wfDef.Name,
			len(wfDef.Steps),
			startedAt,
			fmt.Errorf("workflow %q has no steps defined", wfDef.Name),
			secretValues,
		)
		return
	}

	streamObserver := newChannelObserver(projectMeta.ID, projectMeta.Name, wfDef.ID, wfDef.Name, secretValues, events)
	observer := buildObserver(opts.Observer, streamObserver)

	r := engine.New(s.registry, observer)
	_, runErr := r.RunWithWorkflows(resolveContext(opts.Context), projectMeta, workflows, wfDef)
	s.emitWorkflowFinished(
		events,
		projectMeta.ID,
		projectMeta.Name,
		wfDef.ID,
		wfDef.Name,
		len(wfDef.Steps),
		startedAt,
		runErr,
		secretValues,
	)
}

func (s *Service) emitWorkflowFinished(
	events chan<- model.RunEvent,
	projectID, projectName, workflowID, workflowName string,
	total int,
	startedAt time.Time,
	runErr error,
	secretValues []string,
) {
	event := model.RunEvent{
		Type:         model.EventWorkflowFinished,
		ProjectID:    projectID,
		ProjectName:  projectName,
		WorkflowID:   workflowID,
		WorkflowName: workflowName,
		Total:        total,
		Status:       "ok",
		DurationMS:   time.Since(startedAt).Milliseconds(),
	}
	if runErr != nil {
		event.Status = "failed"
		event.Error = redact.String(runErr.Error(), secretValues)
	}
	events <- event
}

type multiObserver struct {
	first  engine.Observer
	second engine.Observer
}

func (o multiObserver) OnStepStart(event model.StepStartEvent) {
	o.first.OnStepStart(event)
	o.second.OnStepStart(event)
}

func (o multiObserver) OnStepFinish(event model.StepFinishEvent) {
	o.first.OnStepFinish(event)
	o.second.OnStepFinish(event)
}

type channelObserver struct {
	projectID    string
	projectName  string
	workflowID   string
	workflowName string
	secretValues []string
	events       chan<- model.RunEvent
}

func newChannelObserver(
	projectID, projectName,
	workflowID, workflowName string,
	secretValues []string,
	events chan<- model.RunEvent,
) *channelObserver {
	return &channelObserver{
		projectID:    projectID,
		projectName:  projectName,
		workflowID:   workflowID,
		workflowName: workflowName,
		secretValues: secretValues,
		events:       events,
	}
}

func (o *channelObserver) OnStepStart(event model.StepStartEvent) {
	o.events <- model.RunEvent{
		Type:         model.EventStepStarted,
		ProjectID:    o.projectID,
		ProjectName:  o.projectName,
		WorkflowID:   o.workflowID,
		WorkflowName: o.workflowName,
		Index:        event.Index,
		Total:        event.Total,
		Step:         event.Step.Name,
		StepType:     string(event.Step.Type),
		Status:       "running",
	}
}

func (o *channelObserver) OnStepFinish(event model.StepFinishEvent) {
	runEvent := model.RunEvent{
		Type:         model.EventStepFinished,
		ProjectID:    o.projectID,
		ProjectName:  o.projectName,
		WorkflowID:   o.workflowID,
		WorkflowName: o.workflowName,
		Index:        event.Index,
		Total:        event.Total,
		Step:         event.Step.Name,
		StepType:     string(event.Step.Type),
		Status:       event.Status,
		DurationMS:   event.Duration.Milliseconds(),
		Response:     redact.Value(event.Response, o.secretValues),
	}
	if event.Request != nil {
		if m, ok := redact.Value(event.Request, o.secretValues).(map[string]any); ok {
			runEvent.Request = m
		}
	}
	if event.Error != nil {
		runEvent.Error = redact.String(event.Error.Error(), o.secretValues)
	}
	o.events <- runEvent
}

func buildObserver(observer engine.Observer, streamObserver *channelObserver) engine.Observer {
	if observer == nil {
		return streamObserver
	}
	return multiObserver{first: observer, second: streamObserver}
}

func resolveContext(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}
	return context.Background()
}

func secretValuesFromProject(project *model.Project) []string {
	if project == nil {
		return nil
	}

	secrets := make(map[string]string, len(project.Secrets)+len(project.SecretVariables))
	for _, entry := range project.Secrets {
		if entry.Key != "" && entry.Value != "" {
			secrets[entry.Key] = entry.Value
		}
	}
	for _, entry := range project.SecretVariables {
		if entry.Key != "" && entry.Value != "" {
			secrets[entry.Key] = entry.Value
		}
	}
	return redact.SecretValues(secrets)
}
