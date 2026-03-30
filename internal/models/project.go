// Package models contains shared project, workflow, and execution DTOs.
package models

import (
	"fmt"

	"github.com/google/uuid"
)

// Definition holds a loaded project with all its workflows indexed by key.
type Definition struct {
	Meta      *Project
	Workflows map[string]*Workflow
}

// Project is the editable project definition.
type Project struct {
	ID        uuid.UUID
	Name      string
	Constants []Entry
	Variables []Entry
	Secrets   []Entry
}

// Entry is a key/value definition inside a project or workflow.
type Entry struct {
	Key   string
	Value string
}

// Workflow is a chain of executable steps.
type Workflow struct {
	ID           uuid.UUID
	ProjectID    uuid.UUID
	Name         string
	Constants    []Entry
	Variables    []Entry
	Steps        []Step
	OnErrorSteps []Step
}

// Step is one workflow block invocation.
type Step struct {
	ID      uuid.UUID
	Name    string
	Type    string
	Enabled bool
	Config  map[string]any
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
