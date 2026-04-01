package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/wiregoblin/wiregoblin/internal/block"
	condition2 "github.com/wiregoblin/wiregoblin/internal/condition"
	"github.com/wiregoblin/wiregoblin/internal/model"
	"github.com/wiregoblin/wiregoblin/internal/redact"
)

// StepResult is the formatted output for one executed workflow step.
type StepResult struct {
	Index  int    `json:"index"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Status string `json:"status"`
	Output any    `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

type blockLookup interface {
	Get(blockType model.BlockType) (block.Block, error)
}

// Observer receives workflow execution events.
type Observer interface {
	OnStepStart(event model.StepStartEvent)
	OnStepFinish(event model.StepFinishEvent)
}

// Runner executes workflow definitions through the registered blocks.
type Runner struct {
	registry blockLookup
	observer Observer
}

// New creates a workflow runner.
func New(registry blockLookup, observer Observer) *Runner {
	return &Runner{
		registry: registry,
		observer: observer,
	}
}

// maxIterations caps total step executions to prevent infinite goto loops.
const maxIterations = 1000

// Error built-in names injected into RunContext.Builtins when a step fails.
const (
	ErrorVarMessage    = "ErrorMessage"
	ErrorVarBlockID    = "ErrorBlockID"
	ErrorVarBlockName  = "ErrorBlockName"
	ErrorVarBlockType  = "ErrorBlockType"
	ErrorVarBlockIndex = "ErrorBlockIndex"
)

const (
	stepStatusOK           = "ok"
	stepStatusSkipped      = "skipped"
	stepStatusFailed       = "failed"
	stepStatusIgnoredError = "ignored-error"
)

// Run executes all enabled steps sequentially, honouring goto jumps.
// $var, @name, and !builtin references in step configs are resolved from RunContext before each step.
func (r *Runner) Run(
	ctx context.Context,
	project *model.Project,
	definition *model.Workflow,
) ([]StepResult, error) {
	if containsWorkflowStep(definition) {
		return nil, fmt.Errorf("nested workflow steps require RunWithWorkflows")
	}
	return r.RunWithWorkflows(ctx, project, nil, definition)
}

// RunWithWorkflows executes one workflow and exposes sibling definitions for nested workflow steps.
func (r *Runner) RunWithWorkflows(
	ctx context.Context,
	project *model.Project,
	workflows map[string]*model.Workflow,
	definition *model.Workflow,
) ([]StepResult, error) {
	definition = definition.DefaultedCopy()
	if err := model.ValidateWorkflow(definition); err != nil {
		return nil, err
	}
	ctx, cancel := withWorkflowTimeout(ctx, definition)
	defer cancel()

	runCtx := NewRunContext(project, definition)
	r.bindRunContextExecutors(project, workflows, runCtx)
	return r.runWithContext(ctx, runCtx, definition)
}

func (r *Runner) bindRunContextExecutors(
	project *model.Project,
	workflows map[string]*model.Workflow,
	runCtx *block.RunContext,
) {
	if runCtx == nil {
		return
	}

	runCtx.ExecuteWorkflow = func(
		ctx context.Context,
		targetWorkflowID string,
		inputs map[string]string,
	) (*block.WorkflowRunResult, error) {
		target, err := findWorkflowDefinition(workflows, targetWorkflowID)
		if err != nil {
			return nil, err
		}
		ctx, cancel := withWorkflowTimeout(ctx, target)
		defer cancel()

		childCtx := NewRunContext(project, target)
		r.bindRunContextExecutors(project, workflows, childCtx)
		for key, value := range runCtx.Builtins {
			childCtx.Builtins["Parent."+key] = value
		}
		for key, value := range inputs {
			childCtx.Variables[key] = value
		}

		initialVariables := cloneStringMap(childCtx.Variables)
		initialSecretVariables := cloneStringMap(childCtx.SecretVariables)
		results, err := r.runWithContext(ctx, childCtx, target)
		secretValues := runCtxSecretValues(childCtx)
		rawExports := buildWorkflowExports(
			childCtx,
			target,
			initialVariables,
			childCtx.Variables,
			initialSecretVariables,
			childCtx.SecretVariables,
		)
		displayExports := make(map[string]string, len(rawExports))
		for key, value := range rawExports {
			displayExports[strings.TrimPrefix(key, "@")] = value
		}
		runResult := &block.WorkflowRunResult{
			WorkflowID:      target.ID,
			Steps:           toNestedStepResults(results),
			Variables:       redact.Strings(cloneStringMap(childCtx.Variables), secretValues),
			SecretVariables: redact.Strings(cloneStringMap(childCtx.SecretVariables), secretValues),
			Exports:         redact.Strings(displayExports, secretValues),
			RawExports:      rawExports,
		}
		return runResult, err
	}

	runCtx.ExecuteStep = func(ctx context.Context, step model.Step) (*block.Result, error) {
		result, _, _, err := r.executeStep(ctx, runCtx, step, true, true)
		return result, err
	}
	runCtx.ExecuteIsolatedStep = func(
		ctx context.Context,
		isolated *block.RunContext,
		step model.Step,
	) (*block.Result, error) {
		if isolated == nil {
			return nil, fmt.Errorf("isolated run context is required")
		}
		r.bindRunContextExecutors(project, workflows, isolated)
		result, _, _, err := r.executeStep(ctx, isolated, step, true, false)
		return result, err
	}
}

func withWorkflowTimeout(ctx context.Context, definition *model.Workflow) (context.Context, context.CancelFunc) {
	if definition == nil || definition.TimeoutSeconds <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, time.Duration(definition.TimeoutSeconds)*time.Second)
}

func buildWorkflowExports(
	runCtx *block.RunContext,
	definition *model.Workflow,
	_ map[string]string,
	_ map[string]string,
	_ map[string]string,
	_ map[string]string,
) map[string]string {
	if definition != nil && len(definition.Outputs) != 0 {
		exports := make(map[string]string, len(definition.Outputs))
		policy := block.ReferencePolicy{
			Constants:  true,
			Variables:  true,
			InlineOnly: true,
		}
		for key, value := range definition.Outputs {
			exports[key] = block.ResolveReferences(runCtx, value, policy)
		}
		return exports
	}
	return map[string]string{}
}

func (r *Runner) runWithContext(
	ctx context.Context,
	runCtx *block.RunContext,
	definition *model.Workflow,
) ([]StepResult, error) {
	definition = definition.DefaultedCopy()
	results := make([]StepResult, 0, len(definition.Steps))
	stepIDIndex := buildStepIDIndex(definition.Steps)

	index := 0
	iterations := 0
	var runErr error
	for index < len(definition.Steps) {
		iterations++
		if iterations > maxIterations {
			return results, fmt.Errorf("workflow exceeded %d step executions", maxIterations)
		}

		step := definition.Steps[index]
		r.observer.OnStepStart(model.StepStartEvent{
			Index: index + 1,
			Total: len(definition.Steps),
			Step:  step,
		})

		if !step.Enabled {
			r.observer.OnStepFinish(model.StepFinishEvent{
				Index:  index + 1,
				Total:  len(definition.Steps),
				Step:   step,
				Status: stepStatusSkipped,
			})
			results = append(results, StepResult{
				Index:  index + 1,
				Name:   step.Name,
				Type:   string(step.Type),
				Status: stepStatusSkipped,
			})
			index++
			continue
		}

		startedAt := time.Now()
		result, resolved, status, err := r.executeStep(ctx, runCtx, step, false, true)
		duration := time.Since(startedAt)
		if err != nil {
			var response any
			if result != nil {
				response = result.Output
			}
			if step.ContinueOnError {
				r.handleIgnoredStepFailure(
					runCtx,
					definition,
					&results,
					index,
					step,
					resolved.Config,
					response,
					duration,
					err,
				)
				index++
				continue
			}
			runErr = r.handleStepFailure(
				ctx,
				runCtx,
				definition,
				&results,
				index,
				step,
				resolved.Config,
				response,
				duration,
				err,
			)
			break
		}
		if status == stepStatusSkipped {
			var response any
			if result != nil {
				response = result.Output
			}
			r.observer.OnStepFinish(model.StepFinishEvent{
				Index:    index + 1,
				Total:    len(definition.Steps),
				Step:     step,
				Status:   stepStatusSkipped,
				Duration: duration,
				Response: redactValue(runCtx, response),
			})
			results = append(results, newStepResult(index, step, stepStatusSkipped, redactValue(runCtx, response), ""))
			index++
			continue
		}

		if result.Jump != nil {
			targetIdx := -1
			found := false
			if result.Jump.TargetStepID != "" {
				targetIdx, found = stepIDIndex[result.Jump.TargetStepID]
			}
			if !found {
				runErr = r.handleStepFailure(
					ctx,
					runCtx,
					definition,
					&results,
					index,
					step,
					resolved.Config,
					result.Output,
					duration,
					fmt.Errorf("goto target %q not found", result.Jump.TargetStepID),
				)
				break
			}
			r.observer.OnStepFinish(model.StepFinishEvent{
				Index:    index + 1,
				Total:    len(definition.Steps),
				Step:     step,
				Status:   stepStatusOK,
				Duration: duration,
				Request:  redactMap(runCtx, resolved.Config),
				Response: redactValue(runCtx, result.Output),
			})
			results = append(results, newStepResult(index, step, stepStatusOK, redactValue(runCtx, result.Output), ""))
			if result.Jump.WaitSeconds > 0 {
				timer := time.NewTimer(time.Duration(result.Jump.WaitSeconds) * time.Second)
				select {
				case <-ctx.Done():
					timer.Stop()
					return results, ctx.Err()
				case <-timer.C:
				}
			}
			index = targetIdx
			continue
		}

		r.observer.OnStepFinish(model.StepFinishEvent{
			Index:    index + 1,
			Total:    len(definition.Steps),
			Step:     step,
			Status:   stepStatusOK,
			Duration: duration,
			Request:  redactMap(runCtx, resolved.Config),
			Response: redactValue(runCtx, result.Output),
		})
		results = append(results, newStepResult(index, step, stepStatusOK, redactValue(runCtx, result.Output), ""))
		index++
	}

	return results, runErr
}

func (r *Runner) executeStep(
	ctx context.Context,
	runCtx *block.RunContext,
	step model.Step,
	nested bool,
	applyAssignments bool,
) (*block.Result, model.Step, string, error) {
	if result, status, err := evaluateStepCondition(runCtx, step); result != nil || err != nil {
		return result, step, status, err
	}

	blk, err := r.registry.Get(step.Type)
	if err != nil {
		return nil, step, "", err
	}

	stepStart := time.Now()
	runCtx.Builtins["BlockStartTime"] = stepStart.UTC().Format(time.RFC3339)
	runCtx.Builtins["BlockStartUnix"] = fmt.Sprintf("%d", stepStart.Unix())

	resolved := resolveStepConfig(blk, step, runCtx)
	if err := validateStepCapabilities(blk, resolved); err != nil {
		return nil, resolved, "", err
	}
	if err := blk.Validate(resolved); err != nil {
		return nil, resolved, "", err
	}

	result, err := blk.Execute(ctx, runCtx, resolved)
	if result != nil && result.Request != nil {
		resolved.Config = result.Request
	}
	if err != nil {
		return result, resolved, "", err
	}
	if nested && result != nil && result.Jump != nil {
		return result, resolved, "", fmt.Errorf("nested block type %q cannot trigger workflow jumps", step.Type)
	}

	if result != nil {
		runCtx.StepResults[step.ID] = result.Output
		if applyAssignments {
			if err := applyResponseExtractions(runCtx, resolved, result); err != nil {
				return result, resolved, "", err
			}
		}
	}

	return result, resolved, stepStatusOK, nil
}

func findWorkflowDefinition(workflows map[string]*model.Workflow, target string) (*model.Workflow, error) {
	if len(workflows) == 0 {
		return nil, fmt.Errorf("workflow %q not found", target)
	}
	for _, wf := range workflows {
		if wf != nil && wf.ID == target {
			return wf, nil
		}
	}
	return nil, fmt.Errorf(
		"workflow %q not found; available workflow ids: %s",
		target,
		describeAvailableWorkflows(workflows),
	)
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return map[string]string{}
	}
	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func describeAvailableWorkflows(workflows map[string]*model.Workflow) string {
	if len(workflows) == 0 {
		return "none"
	}

	descriptions := make([]string, 0, len(workflows))
	for key, workflow := range workflows {
		if workflow == nil {
			continue
		}
		descriptions = append(descriptions, fmt.Sprintf("%s -> %s", key, workflow.ID))
	}
	slices.Sort(descriptions)
	return strings.Join(descriptions, ", ")
}

func toNestedStepResults(results []StepResult) []block.WorkflowStepResult {
	if len(results) == 0 {
		return nil
	}
	nested := make([]block.WorkflowStepResult, 0, len(results))
	for _, result := range results {
		nested = append(nested, block.WorkflowStepResult{
			Index:  result.Index,
			Name:   result.Name,
			Type:   result.Type,
			Status: result.Status,
			Output: result.Output,
			Error:  result.Error,
		})
	}
	return nested
}

func buildStepIDIndex(steps []model.Step) map[string]int {
	idx := make(map[string]int, len(steps))
	for i, s := range steps {
		if s.ID != "" {
			idx[s.ID] = i
		}
	}
	return idx
}

// resolveStepConfig returns a copy of step with $var and @name references
// in config string values replaced using the current RunContext.
// Response extraction metadata is left unmodified.
func resolveStepConfig(blk block.Block, step model.Step, runCtx *block.RunContext) model.Step {
	resolved := step
	resolved.Config = resolveConfigMap(step.Config, allowedReferencePolicies(blk), runCtx)
	return resolved
}

func resolveConfigMap(
	m map[string]any,
	policies map[string]block.ReferencePolicy,
	runCtx *block.RunContext,
) map[string]any {
	if m == nil {
		return nil
	}
	result := make(map[string]any, len(m))
	for k, v := range m {
		// Leave assignment metadata unmodified — these are variable names and source paths, not values.
		if k == "assign" {
			result[k] = v
			continue
		}
		policy, ok := policies[k]
		if !ok {
			result[k] = v
			continue
		}
		result[k] = resolveConfigValue(v, policy, runCtx)
	}
	return result
}

func resolveConfigValue(v any, policy block.ReferencePolicy, runCtx *block.RunContext) any {
	switch typed := v.(type) {
	case string:
		return block.ResolveReferences(runCtx, typed, policy)
	case map[string]any:
		if !policy.InlineOnly {
			return typed
		}
		result := make(map[string]any, len(typed))
		for k, item := range typed {
			result[k] = resolveConfigValue(item, policy, runCtx)
		}
		return result
	case []any:
		if !policy.InlineOnly {
			return typed
		}
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = resolveConfigValue(item, policy, runCtx)
		}
		return out
	default:
		return v
	}
}

func allowedReferencePolicies(blk block.Block) map[string]block.ReferencePolicy {
	provider, ok := blk.(block.ReferencePolicyProvider)
	if !ok {
		return nil
	}
	policies := provider.ReferencePolicy()
	if len(policies) == 0 {
		return nil
	}
	result := make(map[string]block.ReferencePolicy, len(policies))
	for _, policy := range policies {
		if policy.Field == "" {
			continue
		}
		result[policy.Field] = policy
	}
	return result
}

func validateStepCapabilities(blk block.Block, step model.Step) error {
	if hasExtractionConfig(step.Config) {
		provider, ok := blk.(block.ResponseMappingProvider)
		if !ok || !provider.SupportsResponseMapping() {
			return fmt.Errorf("block type %q does not support assign mappings", string(step.Type))
		}
	}
	return nil
}

func evaluateStepCondition(
	runCtx *block.RunContext,
	step model.Step,
) (*block.Result, string, error) {
	if step.Condition == nil {
		return nil, "", nil
	}

	condition := resolveStepCondition(runCtx, step.Condition)
	variableName, actual, _ := block.ResolveVariableExpression(runCtx, condition.Variable)
	if variableName == "" {
		variableName = condition.Variable
	}

	matched, err := condition2.Evaluate(actual, condition.Operator, condition.Expected)
	if err != nil {
		return &block.Result{Output: map[string]any{
			"condition": map[string]any{
				"variable": variableName,
				"actual":   actual,
				"operator": condition.Operator,
				"expected": condition.Expected,
				"matched":  false,
			},
		}}, "", err
	}

	if matched {
		return nil, "", nil
	}

	return &block.Result{Output: map[string]any{
		"condition": map[string]any{
			"variable": variableName,
			"actual":   actual,
			"operator": condition.Operator,
			"expected": condition.Expected,
			"matched":  false,
		},
	}}, stepStatusSkipped, nil
}

func resolveStepCondition(runCtx *block.RunContext, condition *model.Condition) model.Condition {
	if condition == nil {
		return model.Condition{}
	}

	policy := block.ReferencePolicy{
		Constants:  true,
		Variables:  true,
		InlineOnly: true,
	}

	return model.Condition{
		Variable: strings.TrimSpace(condition.Variable),
		Operator: strings.TrimSpace(condition.Operator),
		Expected: block.ResolveReferences(runCtx, condition.Expected, policy),
	}
}

// applyResponseExtractions writes selected step outputs into the runtime namespace.
func applyResponseExtractions(runCtx *block.RunContext, step model.Step, result *block.Result) error {
	if result == nil {
		return nil
	}
	updates := map[string]string{}
	for key, value := range extractAssignedVariables(result, step.Config) {
		updates[key] = value
	}
	if len(updates) != 0 {
		return applyRuntimeAssignments(runCtx, updates)
	}
	if len(result.Exports) == 0 {
		return nil
	}
	implicitAssignments := make(map[string]string, len(result.Exports))
	for key, value := range result.Exports {
		switch {
		case strings.HasPrefix(key, "$"):
			implicitAssignments[key] = value
		case strings.HasPrefix(key, "@"):
			implicitAssignments["$"+strings.TrimPrefix(key, "@")] = value
		default:
			implicitAssignments["$"+key] = value
		}
	}
	return applyRuntimeAssignments(runCtx, implicitAssignments)
}

type responseMappingEntry struct {
	Key  string
	Path string
}

func decodeResponseEntries(raw any) []responseMappingEntry {
	if entries := decodeResponseShorthand(raw); len(entries) != 0 {
		return entries
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	entries := make([]responseMappingEntry, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		key := strings.TrimSpace(fmt.Sprint(m["key"]))
		path := strings.TrimSpace(fmt.Sprint(m["path"]))
		if key == "" || path == "" {
			continue
		}
		entries = append(entries, responseMappingEntry{Key: key, Path: path})
	}
	return entries
}

func decodeResponseShorthand(raw any) []responseMappingEntry {
	m, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	entries := make([]responseMappingEntry, 0, len(m))
	for target, path := range m {
		key := strings.TrimSpace(target)
		path := strings.TrimSpace(fmt.Sprint(path))
		if key == "" || path == "" {
			continue
		}
		entries = append(entries, responseMappingEntry{Key: key, Path: path})
	}
	return entries
}

func decodeExtractionConfig(config map[string]any, field string) []responseMappingEntry {
	if config == nil {
		return nil
	}
	entries := decodeResponseEntries(config[field])
	if len(entries) == 0 {
		return nil
	}
	return entries
}

func hasExtractionConfig(config map[string]any) bool {
	return len(decodeExtractionConfig(config, "assign")) != 0
}

func (r *Runner) handleStepFailure(
	ctx context.Context,
	runCtx *block.RunContext,
	definition *model.Workflow,
	results *[]StepResult,
	index int,
	step model.Step,
	request map[string]any,
	response any,
	duration time.Duration,
	err error,
) error {
	r.observer.OnStepFinish(model.StepFinishEvent{
		Index:    index + 1,
		Total:    len(definition.Steps),
		Step:     step,
		Status:   stepStatusFailed,
		Duration: duration,
		Request:  redactMap(runCtx, request),
		Response: redactValue(runCtx, response),
		Error:    fmt.Errorf("%s", redactError(runCtx, err)),
	})
	*results = append(*results, newStepResult(index, step, stepStatusFailed, nil, redactError(runCtx, err)))

	runErr := err
	if chainErr := runErrorChain(ctx, r, runCtx, definition, results, index, step, err); chainErr != nil {
		runErr = fmt.Errorf("%w; error handler failed: %v", err, chainErr)
	}

	return runErr
}

func (r *Runner) handleIgnoredStepFailure(
	runCtx *block.RunContext,
	definition *model.Workflow,
	results *[]StepResult,
	index int,
	step model.Step,
	request map[string]any,
	response any,
	duration time.Duration,
	err error,
) {
	r.observer.OnStepFinish(model.StepFinishEvent{
		Index:    index + 1,
		Total:    len(definition.Steps),
		Step:     step,
		Status:   stepStatusIgnoredError,
		Duration: duration,
		Request:  redactMap(runCtx, request),
		Response: redactValue(runCtx, response),
		Error:    fmt.Errorf("%s", redactError(runCtx, err)),
	})
	*results = append(*results, newStepResult(
		index,
		step,
		stepStatusIgnoredError,
		redactValue(runCtx, response),
		redactError(runCtx, err),
	))
}

func newStepResult(index int, step model.Step, status string, output any, err string) StepResult {
	return StepResult{
		Index:  index + 1,
		Name:   step.Name,
		Type:   string(step.Type),
		Status: status,
		Output: output,
		Error:  err,
	}
}

func runCtxSecretValues(runCtx *block.RunContext) []string {
	if runCtx == nil {
		return nil
	}
	secrets := cloneStringMap(runCtx.Secrets)
	for key, value := range runCtx.SecretVariables {
		secrets[key] = value
	}
	return redact.SecretValues(secrets)
}

func redactError(runCtx *block.RunContext, err error) string {
	if err == nil {
		return ""
	}
	return redact.String(err.Error(), runCtxSecretValues(runCtx))
}

func redactMap(runCtx *block.RunContext, value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	redacted, _ := redact.Value(value, runCtxSecretValues(runCtx)).(map[string]any)
	return redacted
}

func redactValue(runCtx *block.RunContext, value any) any {
	return redact.Value(value, runCtxSecretValues(runCtx))
}

func applyRuntimeAssignments(runCtx *block.RunContext, assignments map[string]string) error {
	if runCtx == nil || len(assignments) == 0 {
		return nil
	}

	for target := range assignments {
		if err := validateRuntimeTarget(runCtx, target); err != nil {
			return err
		}
	}
	for target, value := range assignments {
		name := parseRuntimeTarget(target)
		if _, ok := runCtx.SecretVariables[name]; ok {
			runCtx.SecretVariables[name] = value
			continue
		}
		runCtx.Variables[name] = value
	}
	return nil
}

func validateRuntimeTarget(runCtx *block.RunContext, target string) error {
	trimmed := strings.TrimSpace(target)
	if trimmed == "" {
		return fmt.Errorf("runtime target is required")
	}
	if strings.HasPrefix(trimmed, "@") {
		name := strings.TrimSpace(strings.TrimPrefix(trimmed, "@"))
		return fmt.Errorf("runtime target must use $%s, not @%s", name, name)
	}
	if !strings.HasPrefix(trimmed, "$") {
		return fmt.Errorf("runtime target must start with $")
	}
	name := parseRuntimeTarget(trimmed)
	if _, ok := runCtx.Constants[name]; ok {
		return fmt.Errorf("cannot write to read-only constant @%s", name)
	}
	if _, ok := runCtx.Secrets[name]; ok {
		return fmt.Errorf("cannot write to read-only secret @%s", name)
	}
	return nil
}

func parseRuntimeTarget(target string) string {
	trimmed := strings.TrimSpace(target)
	return strings.TrimSpace(strings.TrimPrefix(trimmed, "$"))
}

func resolveKey(m map[string]any, key string) (string, bool) {
	if _, ok := m[key]; ok {
		return key, true
	}
	lower := strings.ToLower(key)
	for k := range m {
		if strings.ToLower(k) == lower {
			return k, true
		}
	}
	return "", false
}

func readMappedValue(obj any, path string) (any, bool) {
	current := obj
	for _, part := range strings.Split(path, ".") {
		if part == "" {
			continue
		}
		m, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		resolved, found := resolveKey(m, part)
		if !found {
			return nil, false
		}
		current = m[resolved]
	}
	return current, true
}

// extractAssignedVariables extracts configured values into runtime variables.
func extractAssignedVariables(result *block.Result, config map[string]any) map[string]string {
	if result == nil {
		return nil
	}
	mappings := decodeExtractionConfig(config, "assign")
	if len(mappings) == 0 {
		return nil
	}
	vars := make(map[string]string, len(mappings))
	for _, m := range mappings {
		value, ok := readAssignedValue(result, m.Path)
		if !ok {
			continue
		}
		var strVal string
		switch v := value.(type) {
		case string:
			strVal = v
		default:
			b, err := json.Marshal(v)
			if err != nil {
				continue
			}
			strVal = string(b)
		}
		vars[m.Key] = strVal
	}
	if len(vars) == 0 {
		return nil
	}
	return vars
}

func readAssignedValue(result *block.Result, path string) (any, bool) {
	path = strings.TrimSpace(path)
	if strings.HasPrefix(path, "outputs.") {
		return readOutputValue(result, path)
	}
	if strings.HasPrefix(path, "body.") {
		return readBodyValue(result, strings.TrimPrefix(path, "body."))
	}
	path = strings.TrimPrefix(strings.TrimSpace(path), "response.")
	if path == "" {
		return nil, false
	}
	if strings.HasPrefix(path, "output.") {
		return readMappedValue(result.Output, strings.TrimPrefix(path, "output."))
	}
	return readMappedValue(result.Output, path)
}

func readBodyValue(result *block.Result, path string) (any, bool) {
	if result == nil {
		return nil, false
	}
	path = strings.TrimSpace(path)
	if path == "" {
		if outputMap, ok := result.Output.(map[string]any); ok {
			if body, found := outputMap["body"]; found {
				return body, true
			}
		}
		return result.Output, result.Output != nil
	}
	if outputMap, ok := result.Output.(map[string]any); ok {
		if body, found := outputMap["body"]; found {
			return readMappedValue(body, path)
		}
	}
	return readMappedValue(result.Output, path)
}

func readOutputValue(result *block.Result, path string) (string, bool) {
	path = strings.TrimPrefix(strings.TrimSpace(path), "outputs.")
	if path == "" {
		return "", false
	}
	value, ok := result.Exports[path]
	return value, ok
}

func runErrorChain(
	ctx context.Context,
	r *Runner,
	runCtx *block.RunContext,
	definition *model.Workflow,
	results *[]StepResult,
	failedIndex int,
	failedStep model.Step,
	failedErr error,
) error {
	blockID := failedStep.BlockID
	if blockID == "" {
		blockID = failedStep.Name
	}
	runCtx.Builtins[ErrorVarMessage] = failedErr.Error()
	runCtx.Builtins[ErrorVarBlockID] = blockID
	runCtx.Builtins[ErrorVarBlockName] = failedStep.Name
	runCtx.Builtins[ErrorVarBlockType] = string(failedStep.Type)
	runCtx.Builtins[ErrorVarBlockIndex] = fmt.Sprintf("%d", failedIndex+1)

	if len(definition.OnErrorSteps) == 0 {
		return nil
	}

	for index, step := range definition.OnErrorSteps {
		stepBlock, err := r.registry.Get(step.Type)
		if err != nil {
			*results = append(*results, newStepResult(index, step, "error-handler-failed", nil, redactError(runCtx, err)))
			return err
		}
		resolved := resolveStepConfig(stepBlock, step, runCtx)
		if err := validateStepCapabilities(stepBlock, resolved); err != nil {
			*results = append(*results, newStepResult(index, step, "error-handler-failed", nil, redactError(runCtx, err)))
			return err
		}
		if err := stepBlock.Validate(resolved); err != nil {
			*results = append(*results, newStepResult(index, step, "error-handler-failed", nil, redactError(runCtx, err)))
			return err
		}
		result, err := stepBlock.Execute(ctx, runCtx, resolved)
		if err != nil {
			*results = append(*results, newStepResult(index, step, "error-handler-failed", nil, redactError(runCtx, err)))
			return err
		}

		runCtx.StepResults["onError:"+step.ID] = result.Output
		if err := applyResponseExtractions(runCtx, resolved, result); err != nil {
			*results = append(*results, newStepResult(index, step, "error-handler-failed", nil, redactError(runCtx, err)))
			return err
		}
		*results = append(*results, newStepResult(index, step, "error-handler-ok", redactValue(runCtx, result.Output), ""))
	}
	return nil
}

func containsWorkflowStep(definition *model.Workflow) bool {
	if definition == nil {
		return false
	}
	for _, step := range definition.Steps {
		if step.Type == model.BlockType("workflow") {
			return true
		}
	}
	return false
}
