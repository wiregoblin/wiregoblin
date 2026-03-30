// Package workflowservice coordinates workflow loading and execution streams.
package workflowservice

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/wiregoblin/wiregoblin/internal/engine"
	"github.com/wiregoblin/wiregoblin/internal/models"
	"github.com/wiregoblin/wiregoblin/internal/redact"
	"github.com/wiregoblin/wiregoblin/internal/repository"
	"github.com/wiregoblin/wiregoblin/internal/runtime"
)

// Service coordinates workflow loading and execution.
type Service struct {
	projects repository.ProjectRepository
}

// RunOptions configures one workflow run.
type RunOptions struct {
	Context  context.Context
	Observer engine.Observer
}

// New creates a workflow service around the provided project repository.
func New(projects repository.ProjectRepository) *Service {
	return &Service{projects: projects}
}

// RunWorkflow validates setup and returns a stream of execution events.
func (s *Service) RunWorkflow(workflowName string, opts RunOptions) (<-chan models.RunEvent, error) {
	project, err := s.projects.GetProject()
	if err != nil {
		return nil, fmt.Errorf("load project: %w", err)
	}

	if len(project.Workflows) == 0 {
		return nil, fmt.Errorf("project %q has no workflows defined", project.Meta.Name)
	}

	wfDef, err := pickWorkflow(project, workflowName)
	if err != nil {
		return nil, err
	}

	events := make(chan models.RunEvent)

	go func() {
		defer close(events)
		s.executeWorkflow(project.Meta, project.Workflows, wfDef, opts, events)
	}()

	return events, nil
}

func (s *Service) executeWorkflow(
	projectMeta *models.Project,
	workflows map[string]*models.Workflow,
	wfDef *models.Workflow,
	opts RunOptions,
	events chan<- models.RunEvent,
) {
	startedAt := time.Now()
	secretValues := secretValuesFromProject(projectMeta)
	events <- models.RunEvent{
		Type:     models.EventWorkflowStarted,
		Project:  projectMeta.Name,
		Workflow: wfDef.Name,
		Total:    len(wfDef.Steps),
		Status:   "running",
	}

	if len(wfDef.Steps) == 0 {
		s.emitWorkflowFinished(
			events,
			projectMeta.Name,
			wfDef.Name,
			len(wfDef.Steps),
			startedAt,
			fmt.Errorf("workflow %q has no steps defined", wfDef.Name),
			secretValues,
		)
		return
	}

	registry := runtime.NewRegistry()
	defer registry.Close()

	streamObserver := newChannelObserver(projectMeta.Name, wfDef.Name, events)
	observer := buildObserver(opts.Observer, streamObserver)

	r := engine.New(registry, engine.WithObserver(observer))
	_, runErr := r.RunWithWorkflows(resolveContext(opts.Context), projectMeta, workflows, wfDef)
	s.emitWorkflowFinished(events, projectMeta.Name, wfDef.Name, len(wfDef.Steps), startedAt, runErr, secretValues)
}

func (s *Service) emitWorkflowFinished(
	events chan<- models.RunEvent,
	projectName, workflowName string,
	total int,
	startedAt time.Time,
	runErr error,
	secretValues []string,
) {
	event := models.RunEvent{
		Type:       models.EventWorkflowFinished,
		Project:    projectName,
		Workflow:   workflowName,
		Total:      total,
		Status:     "ok",
		DurationMS: time.Since(startedAt).Milliseconds(),
	}
	if runErr != nil {
		event.Status = "failed"
		event.Error = redact.String(runErr.Error(), secretValues)
	}
	events <- event
}

func pickWorkflow(project *models.Definition, workflowName string) (*models.Workflow, error) {
	wf, ok := project.Workflows[workflowName]
	if !ok {
		return nil, fmt.Errorf("workflow %q not found; available: %v", workflowName, sortedKeys(project.Workflows))
	}
	return wf, nil
}

type multiObserver struct {
	first  engine.Observer
	second engine.Observer
}

func (o multiObserver) OnStepStart(event engine.StepStartEvent) {
	o.first.OnStepStart(event)
	o.second.OnStepStart(event)
}

func (o multiObserver) OnStepFinish(event engine.StepFinishEvent) {
	o.first.OnStepFinish(event)
	o.second.OnStepFinish(event)
}

type channelObserver struct {
	projectName  string
	workflowName string
	events       chan<- models.RunEvent
}

func newChannelObserver(projectName, workflowName string, events chan<- models.RunEvent) *channelObserver {
	return &channelObserver{
		projectName:  projectName,
		workflowName: workflowName,
		events:       events,
	}
}

func (o *channelObserver) OnStepStart(event engine.StepStartEvent) {
	o.events <- models.RunEvent{
		Type:     models.EventStepStarted,
		Project:  o.projectName,
		Workflow: o.workflowName,
		Index:    event.Index,
		Total:    event.Total,
		Step:     event.Step.Name,
		StepType: event.Step.Type,
		Status:   "running",
	}
}

func (o *channelObserver) OnStepFinish(event engine.StepFinishEvent) {
	runEvent := models.RunEvent{
		Type:       models.EventStepFinished,
		Project:    o.projectName,
		Workflow:   o.workflowName,
		Index:      event.Index,
		Total:      event.Total,
		Step:       event.Step.Name,
		StepType:   event.Step.Type,
		Status:     event.Status,
		DurationMS: event.Duration.Milliseconds(),
		Request:    event.Request,
		Response:   event.Response,
	}
	if event.Error != nil {
		runEvent.Error = event.Error.Error()
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

func sortedKeys(m map[string]*models.Workflow) []string {
	return slices.Sorted(maps.Keys(m))
}

func secretValuesFromProject(project *models.Project) []string {
	if project == nil {
		return nil
	}

	secrets := make(map[string]string, len(project.Secrets))
	for _, entry := range project.Secrets {
		if entry.Key != "" && entry.Value != "" {
			secrets[entry.Key] = entry.Value
		}
	}
	return redact.SecretValues(secrets)
}
