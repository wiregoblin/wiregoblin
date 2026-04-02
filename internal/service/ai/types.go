package ai

// DebugInput contains the failed run context sent to the local model.
type DebugInput struct {
	ProjectID    string         `json:"project_id,omitempty"`
	ProjectName  string         `json:"project_name,omitempty"`
	WorkflowID   string         `json:"workflow_id,omitempty"`
	WorkflowName string         `json:"workflow_name,omitempty"`
	StepIndex    int            `json:"step_index,omitempty"`
	StepName     string         `json:"step_name,omitempty"`
	StepType     string         `json:"step_type,omitempty"`
	Error        string         `json:"error,omitempty"`
	Request      map[string]any `json:"request,omitempty"`
	Response     any            `json:"response,omitempty"`
}

// SuccessInput contains the successful run context sent to the local model.
type SuccessInput struct {
	ProjectID       string           `json:"project_id,omitempty"`
	ProjectName     string           `json:"project_name,omitempty"`
	WorkflowID      string           `json:"workflow_id,omitempty"`
	WorkflowName    string           `json:"workflow_name,omitempty"`
	DurationMS      int64            `json:"duration_ms,omitempty"`
	Passed          int              `json:"passed,omitempty"`
	Skipped         int              `json:"skipped,omitempty"`
	IgnoredError    int              `json:"ignored_error,omitempty"`
	InterestingStep []SuccessfulStep `json:"interesting_steps,omitempty"`
}

// SuccessfulStep is a compact step summary for AI run digests.
type SuccessfulStep struct {
	Name            string `json:"name,omitempty"`
	Type            string `json:"type,omitempty"`
	Status          string `json:"status,omitempty"`
	DurationMS      int64  `json:"duration_ms,omitempty"`
	Error           string `json:"error,omitempty"`
	Attempts        int    `json:"attempts,omitempty"`
	RequestExample  string `json:"request_example,omitempty"`
	ResponseExample string `json:"response_example,omitempty"`
}
