package cliapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/wiregoblin/wiregoblin/internal/model"
	workflowservice "github.com/wiregoblin/wiregoblin/internal/service/workflow"
)

// ExecuteOptions configures one CLI workflow execution.
type ExecuteOptions struct {
	RunOptions workflowservice.RunOptions
	Verbosity  int
	JSONOutput bool
	Stdout     io.Writer
	Stderr     io.Writer
}

// Run runs one workflow if workflowID is set, or all project workflows sequentially if nil.
func (a *App) Run(ctx context.Context, workflowID *string, opts ExecuteOptions) error {
	projectID, err := a.projectID(ctx)
	if err != nil {
		if !opts.JSONOutput {
			_, _ = fmt.Fprintln(opts.Stderr, err)
		}
		return err
	}

	if workflowID != nil {
		return a.executeWorkflow(ctx, projectID, *workflowID, opts)
	}

	names, err := a.service.ListWorkflows(ctx, projectID)
	if err != nil {
		if !opts.JSONOutput {
			_, _ = fmt.Fprintln(opts.Stderr, err)
		}
		return err
	}
	for _, name := range names {
		if err := a.executeWorkflow(ctx, projectID, name, opts); err != nil {
			return err
		}
	}
	return nil
}

// ExecuteWorkflow runs one workflow and renders the event stream for CLI users.
func (a *App) ExecuteWorkflow(ctx context.Context, workflowName string, opts ExecuteOptions) error {
	projectID, err := a.projectID(ctx)
	if err != nil {
		if !opts.JSONOutput {
			_, _ = fmt.Fprintln(opts.Stderr, err)
		}
		return err
	}
	return a.executeWorkflow(ctx, projectID, workflowName, opts)
}

func (a *App) executeWorkflow(ctx context.Context, projectID, workflowName string, opts ExecuteOptions) error {
	events, err := a.service.RunWorkflow(ctx, projectID, workflowName, opts.RunOptions)
	if err != nil {
		if !opts.JSONOutput {
			_, _ = fmt.Fprintln(opts.Stderr, err)
		}
		return err
	}
	return streamWorkflow(events, opts.Verbosity, opts.JSONOutput, opts.Stdout, opts.Stderr)
}

func streamWorkflow(
	events <-chan model.RunEvent,
	verbosity int,
	jsonOutput bool,
	stdout, stderr io.Writer,
) error {
	var runErr error
	text := newTextRenderer(stderr, verbosity)
	for event := range events {
		if jsonOutput {
			if err := printEventJSON(stdout, event); err != nil {
				return err
			}
		} else {
			text.PrintEvent(event)
		}

		if event.Type == model.EventWorkflowFinished && event.Error != "" {
			runErr = errors.New(event.Error)
		}
	}
	return runErr
}

func printEventJSON(out io.Writer, event model.RunEvent) error {
	return json.NewEncoder(out).Encode(event)
}

type textRenderer struct {
	out               io.Writer
	verbosity         int
	activeWorkflowRun int
}

func newTextRenderer(out io.Writer, verbosity int) *textRenderer {
	return &textRenderer{
		out:       out,
		verbosity: verbosity,
	}
}

func (r *textRenderer) PrintEvent(event model.RunEvent) {
	switch event.Type {
	case model.EventWorkflowStarted:
		r.println(r.activeWorkflowRun, renderWorkflowStarted(event))
	case model.EventStepStarted:
		r.println(r.activeWorkflowRun, renderStepStarted(event))
		if event.StepType == "workflow" {
			r.activeWorkflowRun++
		}
	case model.EventStepFinished:
		if event.StepType == "workflow" && r.activeWorkflowRun > 0 {
			r.activeWorkflowRun--
		}
		r.printStepFinished(event)
	case model.EventWorkflowFinished:
		r.println(r.activeWorkflowRun, renderWorkflowFinished(event))
	}
}

func (r *textRenderer) printStepFinished(event model.RunEvent) {
	if event.Status == "ok" && r.verbosity == 0 {
		return
	}
	if event.Status == "skipped" && r.verbosity == 0 {
		return
	}

	r.println(r.activeWorkflowRun, renderStepFinished(event, r.verbosity))

	if r.verbosity >= 2 {
		if summary := summarizeValue(event.Response); summary != "" {
			r.println(r.activeWorkflowRun, fmt.Sprintf("   🧪 Goblin loot: %s", summary))
		}
	}
	if r.verbosity >= 3 {
		if len(event.Request) > 0 {
			r.println(r.activeWorkflowRun, "   📜 Goblin spellbook:")
			r.println(r.activeWorkflowRun, indent("      ", marshalForLog(event.Request)))
		}
		if event.Response != nil {
			r.println(r.activeWorkflowRun, "   💎 Goblin stash:")
			r.println(r.activeWorkflowRun, indent("      ", marshalForLog(event.Response)))
		}
	}
}

func (r *textRenderer) println(depth int, text string) {
	_, _ = fmt.Fprintf(r.out, "%s%s\n", strings.Repeat("  ", depth), text)
}

func renderWorkflowStarted(event model.RunEvent) string {
	if event.Total > 0 {
		return fmt.Sprintf(
			"🧌 Goblin crew enters %q from project %q. %d top-level %s packed.",
			event.WorkflowName,
			event.ProjectName,
			event.Total,
			pluralize(event.Total, "trick", "tricks"),
		)
	}
	return fmt.Sprintf("🧌 Goblin crew enters %q from project %q.", event.WorkflowName, event.ProjectName)
}

func renderStepStarted(event model.RunEvent) string {
	return fmt.Sprintf("[%d/%d] Goblin pokes %q [%s]", event.Index, event.Total, event.Step, event.StepType)
}

func renderStepFinished(event model.RunEvent, verbosity int) string {
	duration := formatDuration(time.Duration(event.DurationMS) * time.Millisecond)

	switch event.Status {
	case "skipped":
		return "   😴 Goblin nap: skipped."
	case "failed":
		if verbosity >= 2 {
			return fmt.Sprintf(
				"   💥 Goblin trap: failed in %s | step=%s type=%s | trouble=%s",
				duration,
				event.Step,
				event.StepType,
				event.Error,
			)
		}
		return fmt.Sprintf("   💥 Goblin trap: failed in %s | trouble=%s", duration, event.Error)
	default:
		if verbosity >= 2 {
			return fmt.Sprintf("   ✅ Goblin loot secured in %s | step=%s type=%s", duration, event.Step, event.StepType)
		}
		return fmt.Sprintf("   ✅ Goblin loot secured in %s", duration)
	}
}

func renderWorkflowFinished(event model.RunEvent) string {
	duration := formatDuration(time.Duration(event.DurationMS) * time.Millisecond)
	if event.Error != "" {
		return fmt.Sprintf("🧌 Goblin raid on %q blew up after %s. Trouble: %s", event.WorkflowName, duration, event.Error)
	}
	if event.Total > 0 {
		return fmt.Sprintf(
			"🧌 Goblin crew hauled %q out of the cave in %s after %d top-level %s.",
			event.WorkflowName,
			duration,
			event.Total,
			pluralize(event.Total, "step", "steps"),
		)
	}
	return fmt.Sprintf("🧌 Goblin crew hauled %q out of the cave in %s.", event.WorkflowName, duration)
}

func pluralize(count int, singular, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
}

func formatDuration(duration time.Duration) string {
	if duration < time.Millisecond {
		return duration.Round(time.Microsecond).String()
	}
	return duration.Round(time.Millisecond).String()
}

func marshalForLog(value any) string {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(body)
}

func indent(prefix, value string) string {
	return prefix + strings.ReplaceAll(value, "\n", "\n"+prefix)
}

func summarizeValue(value any) string {
	if value == nil {
		return ""
	}
	body, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	const maxLen = 160
	text := string(body)
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen-3] + "..."
}
