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
	aiservice "github.com/wiregoblin/wiregoblin/internal/service/ai"
	workflowservice "github.com/wiregoblin/wiregoblin/internal/service/workflow"
)

// ExecuteOptions configures one CLI workflow execution.
type ExecuteOptions struct {
	RunOptions       workflowservice.RunOptions
	Verbosity        int
	JSONOutput       bool
	AISummarySuccess bool
	Stdout           io.Writer
	Stderr           io.Writer
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

	project, err := a.projects.GetProject(ctx, projectID)
	if err != nil {
		if !opts.JSONOutput {
			_, _ = fmt.Fprintln(opts.Stderr, err)
		}
		return err
	}

	for _, workflow := range project.Workflows {
		if workflow == nil || workflow.DisableRun {
			continue
		}
		if err := a.executeWorkflow(ctx, projectID, workflow.ID, opts); err != nil {
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
	project, err := a.projects.GetProject(ctx, projectID)
	if err != nil {
		if !opts.JSONOutput {
			_, _ = fmt.Fprintln(opts.Stderr, err)
		}
		return err
	}

	events, err := a.service.RunWorkflow(ctx, projectID, workflowName, opts.RunOptions)
	if err != nil {
		if !opts.JSONOutput {
			_, _ = fmt.Fprintln(opts.Stderr, err)
		}
		return err
	}

	report, reportErr := streamWorkflowWithReport(events, opts.Verbosity, opts.JSONOutput, opts.Stdout, opts.Stderr)
	if report != nil && project.Meta != nil && project.Meta.AI != nil && project.Meta.AI.Enabled {
		if report.failedWorkflow != nil {
			printAIFailureSummary(ctx, opts.Stderr, project.Meta.AI, report)
		} else if opts.AISummarySuccess && report.workflowFinished != nil && report.workflowFinished.Error == "" {
			printAISuccessSummary(ctx, opts.Stderr, project.Meta.AI, report)
		}
	}
	return reportErr
}

type workflowReport struct {
	steps            []model.RunEvent
	failedStep       *model.RunEvent
	failedWorkflow   *model.RunEvent
	workflowFinished *model.RunEvent
	passed           int
	skipped          int
	ignoredError     int
}

func streamWorkflowWithReport(
	events <-chan model.RunEvent,
	verbosity int,
	jsonOutput bool,
	stdout, stderr io.Writer,
) (*workflowReport, error) {
	var runErr error
	report := &workflowReport{}
	text := newTextRenderer(stderr, verbosity)
	for event := range events {
		if jsonOutput {
			if err := printEventJSON(stdout, event); err != nil {
				return report, err
			}
			flushWriter(stdout)
		} else {
			text.PrintEvent(event)
			flushWriter(stderr)
		}

		if event.Type == model.EventStepFinished && event.Status == "failed" {
			stepCopy := event
			report.failedStep = &stepCopy
		}
		if event.Type == model.EventStepFinished {
			stepCopy := event
			report.steps = append(report.steps, stepCopy)
			switch event.Status {
			case "ok":
				report.passed++
			case "skipped":
				report.skipped++
			case "ignored-error":
				report.ignoredError++
			}
		}
		if event.Type == model.EventWorkflowFinished && event.Error != "" {
			eventCopy := event
			report.failedWorkflow = &eventCopy
			runErr = errors.New(event.Error)
		}
		if event.Type == model.EventWorkflowFinished {
			eventCopy := event
			report.workflowFinished = &eventCopy
		}
	}
	return report, runErr
}

func printEventJSON(out io.Writer, event model.RunEvent) error {
	return json.NewEncoder(out).Encode(event)
}

type textRenderer struct {
	out               io.Writer
	verbosity         int
	activeWorkflowRun int
	stepsPassed       int
	stepsFailed       int
	stepsSkipped      int
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
		r.println(r.activeWorkflowRun, r.renderWorkflowFinished(event))
	}
}

func (r *textRenderer) printStepFinished(event model.RunEvent) {
	if r.activeWorkflowRun == 0 {
		switch event.Status {
		case "ok":
			r.stepsPassed++
		case "failed":
			r.stepsFailed++
		case "skipped":
			r.stepsSkipped++
		}
	}

	if event.Status == "ok" && r.verbosity == 0 && !shouldRenderRetrySummary(event) {
		return
	}
	if event.Status == "skipped" && r.verbosity == 0 {
		return
	}

	r.println(r.activeWorkflowRun, renderStepFinished(event, r.verbosity))

	if r.verbosity == 2 {
		if summary := summarizeValue(event.Response); summary != "" {
			r.println(r.activeWorkflowRun, fmt.Sprintf("   🧪 Goblin loot: %s", summary))
		}
	}
	if r.verbosity >= 1 {
		for _, line := range renderRetryHistory(event) {
			r.println(r.activeWorkflowRun, line)
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
			"🧌 Goblin crew enters %q from project %q. %d %s packed.",
			event.WorkflowName,
			event.ProjectName,
			event.Total,
			pluralize(event.Total, "step", "steps"),
		)
	}
	return fmt.Sprintf("🧌 Goblin crew enters %q from project %q.", event.WorkflowName, event.ProjectName)
}

func renderStepStarted(event model.RunEvent) string {
	if event.Index == 0 || event.Total == 0 {
		return fmt.Sprintf("[-] Goblin pokes %q [%s]", event.Step, event.StepType)
	}
	return fmt.Sprintf("[%d/%d] Goblin pokes %q [%s]", event.Index, event.Total, event.Step, event.StepType)
}

func renderStepFinished(event model.RunEvent, verbosity int) string {
	duration := formatDuration(time.Duration(event.DurationMS) * time.Millisecond)
	attempts := retryAttemptCount(event)

	switch event.Status {
	case "skipped":
		return "   😴 Goblin nap: skipped."
	case "failed":
		if attempts > 1 {
			return fmt.Sprintf("   💥 Goblin trap: failed in %s after %d attempts | trouble=%s", duration, attempts, event.Error)
		}
		return fmt.Sprintf("   💥 Goblin trap: failed in %s | trouble=%s", duration, event.Error)
	default:
		if verbosity >= 2 {
			if vars := assignedVars(event.Request); len(vars) > 0 {
				if attempts > 1 {
					return fmt.Sprintf(
						"   ✅ Goblin loot secured in %s after %d attempts → %s",
						duration,
						attempts,
						strings.Join(vars, ", "),
					)
				}
				return fmt.Sprintf("   ✅ Goblin loot secured in %s → %s", duration, strings.Join(vars, ", "))
			}
		}
		if attempts > 1 {
			return fmt.Sprintf("   ✅ Goblin loot secured in %s after %d attempts", duration, attempts)
		}
		return fmt.Sprintf("   ✅ Goblin loot secured in %s", duration)
	}
}

func shouldRenderRetrySummary(event model.RunEvent) bool {
	return retryAttemptCount(event) > 1
}

func retryAttemptCount(event model.RunEvent) int {
	response, ok := event.Response.(map[string]any)
	if !ok {
		return 0
	}
	switch typed := response["attempts"].(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func renderRetryHistory(event model.RunEvent) []string {
	if event.StepType != "retry" {
		return nil
	}
	response, ok := event.Response.(map[string]any)
	if !ok {
		return nil
	}
	items, ok := response["history"].([]any)
	if !ok || len(items) <= 1 {
		return nil
	}

	lines := make([]string, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		attempt := retryHistoryInt(entry["attempt"])
		retryable, _ := entry["retryable"].(bool)
		nextDelay := retryHistoryInt(entry["next_delay_ms"])
		errText, _ := entry["error"].(string)

		switch {
		case errText == "" && !retryable:
			lines = append(lines, fmt.Sprintf("   ↻ Retry attempt %d succeeded.", attempt))
		case retryable && nextDelay > 0:
			lines = append(lines, fmt.Sprintf("   ↻ Retry attempt %d failed: %s | next delay %dms", attempt, errText, nextDelay))
		case retryable:
			lines = append(lines, fmt.Sprintf("   ↻ Retry attempt %d failed: %s", attempt, errText))
		default:
			lines = append(lines, fmt.Sprintf("   ↻ Retry attempt %d stopped retrying: %s", attempt, errText))
		}
	}
	return lines
}

func retryHistoryInt(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func assignedVars(request map[string]any) []string {
	raw, ok := request["assign"]
	if !ok {
		return nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	vars := make([]string, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if key, ok := m["key"].(string); ok && key != "" {
			vars = append(vars, key)
		}
	}
	return vars
}

func (r *textRenderer) renderWorkflowFinished(event model.RunEvent) string {
	duration := formatDuration(time.Duration(event.DurationMS) * time.Millisecond)
	summary := r.stepSummary()
	if event.Error != "" {
		if summary != "" {
			return fmt.Sprintf("🧌 Goblin raid on %q blew up after %s. %s", event.WorkflowName, duration, summary)
		}
		return fmt.Sprintf("🧌 Goblin raid on %q blew up after %s. Trouble: %s", event.WorkflowName, duration, event.Error)
	}
	if summary != "" {
		return fmt.Sprintf("🧌 Goblin crew hauled %q out of the cave in %s. %s", event.WorkflowName, duration, summary)
	}
	return fmt.Sprintf("🧌 Goblin crew hauled %q out of the cave in %s.", event.WorkflowName, duration)
}

func (r *textRenderer) stepSummary() string {
	total := r.stepsPassed + r.stepsFailed + r.stepsSkipped
	if total == 0 {
		return ""
	}
	if r.stepsFailed > 0 {
		return fmt.Sprintf("💥 %d/%d passed, %d failed.", r.stepsPassed, total, r.stepsFailed)
	}
	if r.stepsSkipped > 0 {
		return fmt.Sprintf("✅ %d/%d passed, %d skipped.", r.stepsPassed, total, r.stepsSkipped)
	}
	return fmt.Sprintf("✅ %d/%d passed.", r.stepsPassed, total)
}

func pluralize(count int, singular, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
}

func formatDuration(duration time.Duration) string {
	if duration == 0 {
		return "< 1ms"
	}
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

func flushWriter(writer io.Writer) {
	type flushError interface {
		Flush() error
	}
	type flushNoError interface {
		Flush()
	}
	switch typed := writer.(type) {
	case flushError:
		_ = typed.Flush()
	case flushNoError:
		typed.Flush()
	}
}

func printAIFailureSummary(ctx context.Context, stderr io.Writer, config *model.AIConfig, report *workflowReport) {
	if config == nil || report == nil || report.failedWorkflow == nil {
		return
	}

	input := aiservice.DebugInput{
		ProjectID:    report.failedWorkflow.ProjectID,
		ProjectName:  report.failedWorkflow.ProjectName,
		WorkflowID:   report.failedWorkflow.WorkflowID,
		WorkflowName: report.failedWorkflow.WorkflowName,
		Error:        report.failedWorkflow.Error,
	}
	if report.failedStep != nil {
		input.StepIndex = report.failedStep.Index
		input.StepName = report.failedStep.Step
		input.StepType = report.failedStep.StepType
		input.Error = report.failedStep.Error
		input.Request = report.failedStep.Request
		input.Response = report.failedStep.Response
	}

	explanation, err := aiservice.ExplainFailure(ctx, config, input)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "🤖 AI summary unavailable: %v\n", err)
		return
	}

	_, _ = fmt.Fprintln(stderr, "🤖 AI summary:")
	_, _ = fmt.Fprintln(stderr, indent("   ", explanation))
	flushWriter(stderr)
}

func printAISuccessSummary(ctx context.Context, stderr io.Writer, config *model.AIConfig, report *workflowReport) {
	if config == nil || report == nil || report.workflowFinished == nil {
		return
	}

	input := aiservice.SuccessInput{
		ProjectID:    report.workflowFinished.ProjectID,
		ProjectName:  report.workflowFinished.ProjectName,
		WorkflowID:   report.workflowFinished.WorkflowID,
		WorkflowName: report.workflowFinished.WorkflowName,
		DurationMS:   report.workflowFinished.DurationMS,
		Passed:       report.passed,
		Skipped:      report.skipped,
		IgnoredError: report.ignoredError,
	}
	for _, step := range interestingSuccessfulSteps(report.steps) {
		input.InterestingStep = append(input.InterestingStep, aiservice.SuccessfulStep{
			Name:            step.Step,
			Type:            step.StepType,
			Status:          step.Status,
			DurationMS:      step.DurationMS,
			Error:           step.Error,
			Attempts:        retryAttemptCount(step),
			RequestExample:  compactExample(step.Request),
			ResponseExample: compactExample(step.Response),
		})
	}

	explanation, err := aiservice.SummarizeSuccess(ctx, config, input)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "🤖 AI summary unavailable: %v\n", err)
		return
	}

	_, _ = fmt.Fprintln(stderr, "🤖 AI summary:")
	_, _ = fmt.Fprintln(stderr, indent("   ", explanation))
	flushWriter(stderr)
}

func interestingSuccessfulSteps(steps []model.RunEvent) []model.RunEvent {
	if len(steps) == 0 {
		return nil
	}

	items := make([]model.RunEvent, 0, len(steps))
	for _, step := range steps {
		if step.Status == "skipped" ||
			step.Status == "ignored-error" ||
			retryAttemptCount(step) > 1 ||
			step.StepType == "workflow" ||
			step.StepType == "parallel" {
			items = append(items, step)
			continue
		}
		if compactExample(step.Request) != "" || compactExample(step.Response) != "" {
			items = append(items, step)
			continue
		}
		if step.DurationMS >= 1000 {
			items = append(items, step)
		}
	}
	if len(items) > 6 {
		return items[:6]
	}
	return items
}

func compactExample(value any) string {
	text := summarizeValue(value)
	if text == "" || text == "null" || text == "{}" || text == "[]" {
		return ""
	}
	return text
}
