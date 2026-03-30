package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/models"
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

// Runner executes workflow definitions through the registered blocks.
type Runner struct {
	registry *Registry
	observer Observer
}

// New creates a workflow runner.
func New(registry *Registry, options ...Option) *Runner {
	runner := &Runner{
		registry: registry,
		observer: nopObserver{},
	}
	for _, option := range options {
		option(runner)
	}
	return runner
}

// maxIterations caps total step executions to prevent infinite goto loops.
const maxIterations = 1000

// Error variable names injected into RunContext.Variables when a step fails.
const (
	ErrorVarMessage    = "ErrorMessage"
	ErrorVarBlockName  = "ErrorBlockName"
	ErrorVarBlockType  = "ErrorBlockType"
	ErrorVarBlockIndex = "ErrorBlockIndex"
)

// Run executes all enabled steps sequentially, honouring goto jumps.
// @var and $const references in step configs are resolved from RunContext before each step.
func (r *Runner) Run(
	ctx context.Context,
	project *models.Project,
	definition *models.Workflow,
) ([]StepResult, error) {
	if containsWorkflowStep(definition) {
		return nil, fmt.Errorf("nested workflow steps require RunWithWorkflows")
	}
	return r.RunWithWorkflows(ctx, project, nil, definition)
}

// RunWithWorkflows executes one workflow and exposes sibling definitions for nested workflow steps.
func (r *Runner) RunWithWorkflows(
	ctx context.Context,
	project *models.Project,
	workflows map[string]*models.Workflow,
	definition *models.Workflow,
) ([]StepResult, error) {
	definition = definition.DefaultedCopy()
	if err := models.ValidateWorkflow(definition); err != nil {
		return nil, err
	}

	runCtx := NewRunContext(project, definition)
	runCtx.ExecuteWorkflow = func(
		ctx context.Context,
		targetWorkflowUID string,
		inputs map[string]string,
	) (*block.WorkflowRunResult, error) {
		target, err := findWorkflowDefinition(workflows, targetWorkflowUID)
		if err != nil {
			return nil, err
		}

		childCtx := NewRunContext(project, target)
		childCtx.ExecuteWorkflow = runCtx.ExecuteWorkflow
		for key, value := range inputs {
			childCtx.Variables[key] = value
		}

		initialVariables := cloneStringMap(childCtx.Variables)
		results, err := r.runWithContext(ctx, childCtx, target)
		secretValues := runCtxSecretValues(childCtx)
		rawExports := diffStringMap(initialVariables, childCtx.Variables)
		runResult := &block.WorkflowRunResult{
			WorkflowUID: target.ID.String(),
			Workflow:    target.Name,
			Steps:       toNestedStepResults(results),
			Variables:   redact.Strings(cloneStringMap(childCtx.Variables), secretValues),
			Exports:     redact.Strings(rawExports, secretValues),
			RawExports:  rawExports,
		}
		return runResult, err
	}
	return r.runWithContext(ctx, runCtx, definition)
}

func (r *Runner) runWithContext(
	ctx context.Context,
	runCtx *block.RunContext,
	definition *models.Workflow,
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
		r.observer.OnStepStart(StepStartEvent{
			Index: index + 1,
			Total: len(definition.Steps),
			Step:  step,
		})

		if !step.Enabled {
			r.observer.OnStepFinish(StepFinishEvent{
				Index:  index + 1,
				Total:  len(definition.Steps),
				Step:   step,
				Status: "skipped",
			})
			results = append(results, StepResult{
				Index:  index + 1,
				Name:   step.Name,
				Type:   step.Type,
				Status: "skipped",
			})
			index++
			continue
		}

		block, err := r.registry.MustGet(step.Type)
		if err != nil {
			runErr = r.handleStepFailure(
				ctx,
				runCtx,
				definition,
				&results,
				index,
				step,
				nil,
				nil,
				0,
				err,
			)
			break
		}

		// Resolve @var and $const references in step config before validation and execution.
		resolved := resolveStepConfig(block, step, runCtx)
		if err := validateStepCapabilities(block, resolved); err != nil {
			runErr = r.handleStepFailure(
				ctx,
				runCtx,
				definition,
				&results,
				index,
				step,
				resolved.Config,
				nil,
				0,
				err,
			)
			break
		}

		if err := block.Validate(resolved); err != nil {
			runErr = r.handleStepFailure(
				ctx,
				runCtx,
				definition,
				&results,
				index,
				step,
				resolved.Config,
				nil,
				0,
				err,
			)
			break
		}

		startedAt := time.Now()
		result, err := block.Execute(ctx, runCtx, resolved)
		duration := time.Since(startedAt)
		if err != nil {
			var response any
			if result != nil {
				response = result.Output
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

		runCtx.StepResults[step.Name] = result.Output
		applyExports(runCtx, resolved, result)
		applyResponseMapping(runCtx, resolved, result)

		if result.Jump != nil {
			targetIdx := -1
			found := false
			if result.Jump.TargetStepUID != "" {
				targetIdx, found = stepIDIndex[result.Jump.TargetStepUID]
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
					fmt.Errorf("goto target %q not found", result.Jump.TargetStepUID),
				)
				break
			}
			r.observer.OnStepFinish(StepFinishEvent{
				Index:    index + 1,
				Total:    len(definition.Steps),
				Step:     step,
				Status:   "ok",
				Duration: duration,
				Request:  redactMap(runCtx, resolved.Config),
				Response: redactValue(runCtx, result.Output),
			})
			results = append(results, newStepResult(index, step, "ok", redactValue(runCtx, result.Output), ""))
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

		r.observer.OnStepFinish(StepFinishEvent{
			Index:    index + 1,
			Total:    len(definition.Steps),
			Step:     step,
			Status:   "ok",
			Duration: duration,
			Request:  redactMap(runCtx, resolved.Config),
			Response: redactValue(runCtx, result.Output),
		})
		results = append(results, newStepResult(index, step, "ok", redactValue(runCtx, result.Output), ""))
		index++
	}

	return results, runErr
}

func findWorkflowDefinition(workflows map[string]*models.Workflow, target string) (*models.Workflow, error) {
	if len(workflows) == 0 {
		return nil, fmt.Errorf("workflow %q not found", target)
	}
	for _, wf := range workflows {
		if wf != nil && wf.ID.String() == target {
			return wf, nil
		}
	}
	return nil, fmt.Errorf(
		"workflow %q not found; target_workflow_uid expects a workflow UUID, available: %s",
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

func describeAvailableWorkflows(workflows map[string]*models.Workflow) string {
	if len(workflows) == 0 {
		return "none"
	}

	descriptions := make([]string, 0, len(workflows))
	for key, workflow := range workflows {
		if workflow == nil {
			continue
		}
		descriptions = append(descriptions, fmt.Sprintf("%s -> %s", key, workflow.ID.String()))
	}
	slices.Sort(descriptions)
	return strings.Join(descriptions, ", ")
}

func diffStringMap(before, after map[string]string) map[string]string {
	if len(after) == 0 {
		return nil
	}
	diff := make(map[string]string)
	for key, value := range after {
		if previous, ok := before[key]; !ok || previous != value {
			diff[key] = value
		}
	}
	if len(diff) == 0 {
		return nil
	}
	return diff
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

func buildStepIDIndex(steps []models.Step) map[string]int {
	idx := make(map[string]int, len(steps))
	for i, s := range steps {
		if s.ID.String() != "" {
			idx[s.ID.String()] = i
		}
	}
	return idx
}

// resolveStepConfig returns a copy of step with @var and $const references
// in config string values replaced using the current RunContext.
// The response_mapping and outputs keys are left unmodified.
func resolveStepConfig(blk block.Block, step models.Step, runCtx *block.RunContext) models.Step {
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
		// Leave output mapping metadata unmodified — these are variable names, not values.
		if k == "response_mapping" || k == "outputs" {
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
		return resolveRefs(typed, policy, runCtx)
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

func resolveRefs(s string, policy block.ReferencePolicy, runCtx *block.RunContext) string {
	if runCtx == nil {
		return s
	}
	var builder strings.Builder
	builder.Grow(len(s))

	for i := 0; i < len(s); {
		if !isReferencePrefix(s[i]) {
			builder.WriteByte(s[i])
			i++
			continue
		}

		prefix := s[i]
		start := i + 1
		end := start
		for end < len(s) && isReferenceNameChar(s[end]) {
			end++
		}
		if start == end {
			builder.WriteByte(s[i])
			i++
			continue
		}

		name := s[start:end]
		if resolved, ok := lookupReference(prefix, name, policy, runCtx); ok {
			builder.WriteString(resolved)
		} else {
			builder.WriteByte(prefix)
			builder.WriteString(name)
		}
		i = end
	}

	return builder.String()
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

func validateStepCapabilities(blk block.Block, step models.Step) error {
	if step.Config["response_mapping"] != nil {
		provider, ok := blk.(block.ResponseMappingProvider)
		if !ok || !provider.SupportsResponseMapping() {
			return fmt.Errorf("block type %q does not support response mapping", step.Type)
		}
	}
	return nil
}

// applyExports writes block exports into RunContext.Variables.
func applyExports(runCtx *block.RunContext, step models.Step, result *block.Result) {
	if len(result.Exports) == 0 {
		return
	}
	if mapping := decodeOutputsMapping(step.Config["outputs"]); mapping != nil {
		for varName, exportKey := range mapping {
			if val, ok := result.Exports[exportKey]; ok {
				runCtx.Variables[varName] = val
			}
		}
		return
	}
	for k, v := range result.Exports {
		runCtx.Variables[k] = v
	}
}

// decodeOutputsMapping coerces an "outputs" config value into map[string]string.
func decodeOutputsMapping(raw any) map[string]string {
	switch v := raw.(type) {
	case map[string]string:
		return v
	case map[string]any:
		m := make(map[string]string, len(v))
		for key, val := range v {
			m[key] = fmt.Sprint(val)
		}
		return m
	case nil:
		return nil
	default:
		return nil
	}
}

// applyResponseMapping extracts values from result.Output using dot-path rules.
func applyResponseMapping(runCtx *block.RunContext, step models.Step, result *block.Result) {
	if result.Output == nil {
		return
	}
	vars := ExtractMappedVariables(result.Output, step.Config["response_mapping"])
	for k, v := range vars {
		runCtx.Variables[k] = v
	}
}

type responseMappingEntry struct {
	Key  string
	Path string
}

func decodeResponseMapping(raw any) []responseMappingEntry {
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

func (r *Runner) handleStepFailure(
	ctx context.Context,
	runCtx *block.RunContext,
	definition *models.Workflow,
	results *[]StepResult,
	index int,
	step models.Step,
	request map[string]any,
	response any,
	duration time.Duration,
	err error,
) error {
	r.observer.OnStepFinish(StepFinishEvent{
		Index:    index + 1,
		Total:    len(definition.Steps),
		Step:     step,
		Status:   "failed",
		Duration: duration,
		Request:  redactMap(runCtx, request),
		Response: redactValue(runCtx, response),
		Error:    fmt.Errorf("%s", redactError(runCtx, err)),
	})
	*results = append(*results, newStepResult(index, step, "failed", nil, redactError(runCtx, err)))

	runErr := err
	if chainErr := runErrorChain(ctx, r, runCtx, definition, results, index, step, err); chainErr != nil {
		runErr = fmt.Errorf("%w; error handler failed: %v", err, chainErr)
	}

	return runErr
}

func newStepResult(index int, step models.Step, status string, output any, err string) StepResult {
	return StepResult{
		Index:  index + 1,
		Name:   step.Name,
		Type:   step.Type,
		Status: status,
		Output: output,
		Error:  err,
	}
}

func runCtxSecretValues(runCtx *block.RunContext) []string {
	if runCtx == nil {
		return nil
	}
	return redact.SecretValues(runCtx.Secrets)
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

func isReferencePrefix(ch byte) bool {
	return ch == '@' || ch == '$'
}

func isReferenceNameChar(ch byte) bool {
	return ch == '_' || ch == '-' ||
		(ch >= 'a' && ch <= 'z') ||
		(ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9')
}

func lookupReference(
	prefix byte,
	name string,
	policy block.ReferencePolicy,
	runCtx *block.RunContext,
) (string, bool) {
	switch prefix {
	case '@':
		if !policy.Variables {
			return "", false
		}
		value, ok := runCtx.Variables[name]
		return value, ok
	case '$':
		if !policy.Constants {
			return "", false
		}
		value, ok := runCtx.Constants[name]
		return value, ok
	default:
		return "", false
	}
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

// ExtractMappedVariables extracts response-mapping variable values from output.
func ExtractMappedVariables(output any, rawMapping any) map[string]string {
	if output == nil {
		return nil
	}
	mappings := decodeResponseMapping(rawMapping)
	if len(mappings) == 0 {
		return nil
	}
	result := make(map[string]string, len(mappings))
	for _, m := range mappings {
		path := strings.TrimPrefix(m.Path, "response.")
		value, ok := readMappedValue(output, path)
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
		result[m.Key] = strVal
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// MarshalResults formats step results as indented JSON.
func MarshalResults(results []StepResult) (string, error) {
	body, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal workflow results: %w", err)
	}
	return string(body), nil
}

func runErrorChain(
	ctx context.Context,
	r *Runner,
	runCtx *block.RunContext,
	definition *models.Workflow,
	results *[]StepResult,
	failedIndex int,
	failedStep models.Step,
	failedErr error,
) error {
	runCtx.Variables[ErrorVarMessage] = failedErr.Error()
	runCtx.Variables[ErrorVarBlockName] = failedStep.Name
	runCtx.Variables[ErrorVarBlockType] = failedStep.Type
	runCtx.Variables[ErrorVarBlockIndex] = fmt.Sprintf("%d", failedIndex+1)

	if len(definition.OnErrorSteps) == 0 {
		return nil
	}

	for index, step := range definition.OnErrorSteps {
		block, err := r.registry.MustGet(step.Type)
		if err != nil {
			*results = append(*results, newStepResult(index, step, "error-handler-failed", nil, redactError(runCtx, err)))
			return err
		}
		resolved := resolveStepConfig(block, step, runCtx)
		if err := validateStepCapabilities(block, resolved); err != nil {
			*results = append(*results, newStepResult(index, step, "error-handler-failed", nil, redactError(runCtx, err)))
			return err
		}
		if err := block.Validate(resolved); err != nil {
			*results = append(*results, newStepResult(index, step, "error-handler-failed", nil, redactError(runCtx, err)))
			return err
		}
		result, err := block.Execute(ctx, runCtx, resolved)
		if err != nil {
			*results = append(*results, newStepResult(index, step, "error-handler-failed", nil, redactError(runCtx, err)))
			return err
		}

		runCtx.StepResults["onError:"+step.Name] = result.Output
		applyExports(runCtx, resolved, result)
		applyResponseMapping(runCtx, resolved, result)
		*results = append(*results, newStepResult(index, step, "error-handler-ok", redactValue(runCtx, result.Output), ""))
	}
	return nil
}

func containsWorkflowStep(definition *models.Workflow) bool {
	if definition == nil {
		return false
	}
	for _, step := range definition.Steps {
		if step.Type == "workflow" {
			return true
		}
	}
	return false
}
