// Package model contains shared project, workflow, and execution DTOs.
package model

import (
	"fmt"
)

// BlockType identifies one workflow block implementation.
type BlockType string

// Definition holds a loaded project with all its workflows indexed by key.
type Definition struct {
	Meta      *Project
	Workflows map[string]*Workflow
}

// Project is the editable project definition.
type Project struct {
	ID              string
	Name            string
	Constants       []Entry
	Variables       []Entry
	Secrets         []Entry
	SecretVariables []Entry
}

// Entry is a key/value definition inside a project or workflow.
type Entry struct {
	Key   string
	Value string
}

// Workflow is a chain of executable steps.
type Workflow struct {
	ID              string
	ProjectID       string
	Name            string
	TimeoutSeconds  int
	Constants       []Entry
	Outputs         map[string]string
	Variables       []Entry
	Secrets         []Entry
	SecretVariables []Entry
	Steps           []Step
	OnErrorSteps    []Step
}

// Step is one workflow block invocation.
type Step struct {
	ID              string
	BlockID         string
	Name            string
	Type            BlockType
	Enabled         bool
	ContinueOnError bool
	Condition       *Condition
	Config          map[string]any
}

// Condition describes one reusable comparison gate for a step.
type Condition struct {
	Variable string
	Operator string
	Expected string
}

// EnsureBlockDefaults normalizes a step's generic fields before execution.
func (s *Step) EnsureBlockDefaults(index int) {
	if s.Name == "" {
		s.Name = fmt.Sprintf("Step %d", index+1)
	}
	if s.Config == nil {
		s.Config = map[string]any{}
	}
}

// DefaultedCopy returns a copy with default generic fields populated.
func (s Step) DefaultedCopy(index int) Step {
	s.EnsureBlockDefaults(index)
	return s
}

// DefaultedCopy returns a copy with step defaults populated.
func (w *Workflow) DefaultedCopy() *Workflow {
	if w == nil {
		return nil
	}

	copyWorkflow := *w
	copyWorkflow.Steps = make([]Step, len(w.Steps))
	for index, step := range w.Steps {
		copyWorkflow.Steps[index] = step.DefaultedCopy(index)
	}
	copyWorkflow.OnErrorSteps = make([]Step, len(w.OnErrorSteps))
	for index, step := range w.OnErrorSteps {
		copyWorkflow.OnErrorSteps[index] = step.DefaultedCopy(index)
	}

	return &copyWorkflow
}
