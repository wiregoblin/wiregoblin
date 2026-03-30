package engine

import (
	"time"

	"github.com/wiregoblin/wiregoblin/internal/models"
)

// Observer receives workflow execution events.
type Observer interface {
	OnStepStart(event StepStartEvent)
	OnStepFinish(event StepFinishEvent)
}

// StepStartEvent is emitted before a step starts executing.
type StepStartEvent struct {
	Index int
	Total int
	Step  models.Step
}

// StepFinishEvent is emitted after a step finishes or is skipped.
type StepFinishEvent struct {
	Index    int
	Total    int
	Step     models.Step
	Status   string
	Duration time.Duration
	Request  map[string]any
	Response any
	Error    error
}

type nopObserver struct{}

func (nopObserver) OnStepStart(StepStartEvent)   {}
func (nopObserver) OnStepFinish(StepFinishEvent) {}

// Option configures the workflow runner.
type Option func(*Runner)

// WithObserver attaches an execution observer to the runner.
func WithObserver(observer Observer) Option {
	return func(r *Runner) {
		if observer != nil {
			r.observer = observer
		}
	}
}
