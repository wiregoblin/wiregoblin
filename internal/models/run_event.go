package models

// Workflow run event types emitted while a workflow executes.
const (
	EventStepStarted      = "step_started"
	EventStepFinished     = "step_finished"
	EventWorkflowStarted  = "workflow_started"
	EventWorkflowFinished = "workflow_finished"
)

// RunEvent is one streamed workflow execution event.
type RunEvent struct {
	Type       string         `json:"type"`
	Project    string         `json:"project,omitempty"`
	Workflow   string         `json:"workflow,omitempty"`
	Index      int            `json:"index,omitempty"`
	Total      int            `json:"total,omitempty"`
	Step       string         `json:"step,omitempty"`
	StepType   string         `json:"step_type,omitempty"`
	Status     string         `json:"status,omitempty"`
	DurationMS int64          `json:"duration_ms,omitempty"`
	Request    map[string]any `json:"request,omitempty"`
	Response   any            `json:"response,omitempty"`
	Error      string         `json:"error,omitempty"`
}
