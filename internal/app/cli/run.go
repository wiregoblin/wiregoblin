package cliapp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/wiregoblin/wiregoblin/internal/models"
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

// ExecuteWorkflow runs one workflow and renders the event stream for CLI users.
func (a *App) ExecuteWorkflow(workflowName string, opts ExecuteOptions) error {
	secretValues := a.secretValues()
	events, err := a.RunWorkflow(workflowName, opts.RunOptions)
	if err != nil {
		if !opts.JSONOutput {
			_, _ = fmt.Fprintln(opts.Stderr, err)
		}
		return err
	}
	return streamWorkflow(
		events,
		secretValues,
		opts.Verbosity,
		opts.JSONOutput,
		opts.Stdout,
		opts.Stderr,
	)
}

func streamWorkflow(
	events <-chan models.RunEvent,
	secretValues []string,
	verbosity int,
	jsonOutput bool,
	stdout, stderr io.Writer,
) error {
	var runErr error
	text := newTextRenderer(stderr, verbosity)
	for event := range events {
		event = redactRunEvent(event, secretValues)
		if jsonOutput {
			if err := printEventJSON(stdout, event); err != nil {
				return err
			}
		} else {
			text.PrintEvent(event)
		}

		if event.Type == models.EventWorkflowFinished && event.Error != "" {
			runErr = errors.New(event.Error)
		}
	}
	return runErr
}

func (a *App) secretValues() []string {
	if a.projects == nil {
		return nil
	}

	project, err := a.projects.GetProject()
	if err != nil || project == nil || project.Meta == nil {
		return nil
	}

	values := make([]string, 0, len(project.Meta.Secrets))
	for _, entry := range project.Meta.Secrets {
		if entry.Value != "" {
			values = append(values, entry.Value)
		}
	}

	slices.Sort(values)
	return slices.Compact(values)
}

func redactRunEvent(event models.RunEvent, secretValues []string) models.RunEvent {
	if len(secretValues) == 0 {
		return event
	}

	event.Error = redactString(event.Error, secretValues)
	event.Request = redactMap(event.Request, secretValues)
	event.Response = redactValue(event.Response, secretValues)
	return event
}

func redactMap(input map[string]any, secretValues []string) map[string]any {
	if input == nil {
		return nil
	}

	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = redactValue(value, secretValues)
	}
	return output
}

func redactValue(value any, secretValues []string) any {
	switch typed := value.(type) {
	case string:
		return redactString(typed, secretValues)
	case map[string]any:
		return redactMap(typed, secretValues)
	case []any:
		items := make([]any, len(typed))
		for index, item := range typed {
			items[index] = redactValue(item, secretValues)
		}
		return items
	default:
		return value
	}
}

func redactString(value string, secretValues []string) string {
	redacted := value
	for _, secret := range secretValues {
		if secret == "" {
			continue
		}
		redacted = strings.ReplaceAll(redacted, secret, "[REDACTED]")
	}
	return redacted
}

func printEventJSON(out io.Writer, event models.RunEvent) error {
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

func (r *textRenderer) PrintEvent(event models.RunEvent) {
	switch event.Type {
	case models.EventWorkflowStarted:
		r.println(r.activeWorkflowRun, renderWorkflowStarted(event))
	case models.EventStepStarted:
		r.println(r.activeWorkflowRun, renderStepStarted(event))
		if event.StepType == "workflow" {
			r.activeWorkflowRun++
		}
	case models.EventStepFinished:
		if event.StepType == "workflow" && r.activeWorkflowRun > 0 {
			r.activeWorkflowRun--
		}
		r.printStepFinished(event)
	case models.EventWorkflowFinished:
		r.println(r.activeWorkflowRun, renderWorkflowFinished(event))
	}
}

func (r *textRenderer) printStepFinished(event models.RunEvent) {
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

func renderWorkflowStarted(event models.RunEvent) string {
	if event.Total > 0 {
		return fmt.Sprintf(
			"🧌 Goblin crew enters %q from project %q. %d top-level %s packed.",
			event.Workflow,
			event.Project,
			event.Total,
			pluralize(event.Total, "trick", "tricks"),
		)
	}
	return fmt.Sprintf("🧌 Goblin crew enters %q from project %q.", event.Workflow, event.Project)
}

func renderStepStarted(event models.RunEvent) string {
	return fmt.Sprintf("[%d/%d] Goblin pokes %q [%s]", event.Index, event.Total, event.Step, event.StepType)
}

func renderStepFinished(event models.RunEvent, verbosity int) string {
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

func renderWorkflowFinished(event models.RunEvent) string {
	duration := formatDuration(time.Duration(event.DurationMS) * time.Millisecond)
	if event.Error != "" {
		return fmt.Sprintf("🧌 Goblin raid on %q blew up after %s. Trouble: %s", event.Workflow, duration, event.Error)
	}
	if event.Total > 0 {
		return fmt.Sprintf(
			"🧌 Goblin crew hauled %q out of the cave in %s after %d top-level %s.",
			event.Workflow,
			duration,
			event.Total,
			pluralize(event.Total, "step", "steps"),
		)
	}
	return fmt.Sprintf("🧌 Goblin crew hauled %q out of the cave in %s.", event.Workflow, duration)
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
