package model

import "time"

// StepStartEvent is emitted before a step starts executing.
type StepStartEvent struct {
	Index int
	Total int
	Step  Step
}

// StepFinishEvent is emitted after a step finishes or is skipped.
type StepFinishEvent struct {
	Index    int
	Total    int
	Step     Step
	Status   string
	Duration time.Duration
	Request  map[string]any
	Response any
	Error    error
}
