// Package filerepository loads project definitions from YAML files on disk.
package filerepository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/wiregoblin/wiregoblin/internal/model"
)

// Repository loads project definitions from one YAML file on disk.
type Repository struct {
	path string
}

// New creates a file-backed project repository.
func New(path string) *Repository {
	return &Repository{path: path}
}

// ProjectID loads the project file and returns the project's ID.
func (r *Repository) ProjectID(ctx context.Context) (string, error) {
	def, err := r.GetProject(ctx, "")
	if err != nil {
		return "", err
	}
	return def.Meta.ID, nil
}

// GetProject loads and parses one project definition from disk.
// projectID is ignored since the file repository is scoped to a single project.
func (r *Repository) GetProject(_ context.Context, _ string) (*model.Definition, error) {
	data, err := os.ReadFile(r.path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", r.path, err)
	}
	return parse(data)
}

// GetWorkflow returns the workflow with the given ID from the project.
// projectID is ignored since the file repository is scoped to a single project.
func (r *Repository) GetWorkflow(ctx context.Context, _ string, workflowID string) (*model.Workflow, error) {
	def, err := r.GetProject(ctx, "")
	if err != nil {
		return nil, err
	}
	wf, ok := def.WorkflowByID[workflowID]
	if !ok {
		return nil, fmt.Errorf("workflow %q not found; available: %v", workflowID, orderedWorkflowIDs(def.Workflows))
	}
	return wf, nil
}

// ListWorkflows returns all workflow IDs defined in the project in config order.
// projectID is ignored since the file repository is scoped to a single project.
func (r *Repository) ListWorkflows(ctx context.Context, _ string) ([]string, error) {
	def, err := r.GetProject(ctx, "")
	if err != nil {
		return nil, err
	}
	return orderedWorkflowIDs(def.Workflows), nil
}

type rawConfig struct {
	ID              string            `yaml:"id"`
	Name            string            `yaml:"name"`
	Version         int               `yaml:"version"`
	AI              *rawAIConfig      `yaml:"ai"`
	Constants       map[string]string `yaml:"constants"`
	Secrets         map[string]string `yaml:"secrets"`
	Variables       yaml.Node         `yaml:"variables"`
	SecretVariables yaml.Node         `yaml:"secret_variables"`
	Workflows       rawWorkflows      `yaml:"workflows"`
}

type rawAIConfig struct {
	Enabled        *bool  `yaml:"enabled"`
	Provider       string `yaml:"provider"`
	BaseURL        string `yaml:"base_url"`
	Model          string `yaml:"model"`
	TimeoutSeconds int    `yaml:"timeout_seconds"`
	RedactSecrets  *bool  `yaml:"redact_secrets"`
}

type rawWorkflow struct {
	ID               string            `yaml:"id"`
	Name             string            `yaml:"name"`
	DisableRun       bool              `yaml:"disable_run"`
	TimeoutSeconds   int               `yaml:"timeout_seconds"`
	Constants        map[string]string `yaml:"constants"`
	Secrets          map[string]string `yaml:"secrets"`
	Outputs          map[string]string `yaml:"outputs"`
	Variables        yaml.Node         `yaml:"variables"`
	SecretVariables  yaml.Node         `yaml:"secret_variables"`
	Blocks           orderedBlocks     `yaml:"blocks"`
	CatchErrorBlocks orderedBlocks     `yaml:"catch_error_blocks"`
}

type rawWorkflows []rawWorkflow

const legacyWorkflowMapFormatError = "" +
	"workflows must be a sequence like 'workflows: [{id: ...}]' " +
	"or YAML list items with '- id:'; map form is no longer supported"

func (w *rawWorkflows) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode && value.Value == "" {
		*w = nil
		return nil
	}
	if value.Kind == yaml.MappingNode {
		return errors.New(legacyWorkflowMapFormatError)
	}
	if value.Kind != yaml.SequenceNode {
		return fmt.Errorf("workflows must be a sequence")
	}

	var items []rawWorkflow
	if err := value.Decode(&items); err != nil {
		return err
	}
	*w = items
	return nil
}

type blockEntry struct {
	ID     string
	Values map[string]any
}

// orderedBlocks preserves YAML sequence order during unmarshalling.
type orderedBlocks struct {
	Items []blockEntry
}

func (b *orderedBlocks) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode && value.Value == "" {
		return nil
	}
	if value.Kind != yaml.SequenceNode {
		return fmt.Errorf("expected a sequence for blocks, got %v", value.Kind)
	}
	b.Items = make([]blockEntry, 0, len(value.Content))
	for index, child := range value.Content {
		var raw map[string]any
		if err := child.Decode(&raw); err != nil {
			return fmt.Errorf("decode block %d: %w", index+1, err)
		}
		id := strings.TrimSpace(fmt.Sprint(raw["id"]))
		if id == "" {
			return fmt.Errorf("block %d id is required", index+1)
		}
		delete(raw, "id")
		b.Items = append(b.Items, blockEntry{ID: id, Values: raw})
	}
	return nil
}

func parse(data []byte) (*model.Definition, error) {
	var raw rawConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}

	projectID := strings.TrimSpace(raw.ID)
	if projectID == "" {
		projectID = raw.Name
	}

	meta := &model.Project{
		ID:   projectID,
		Name: raw.Name,
	}
	if raw.AI != nil {
		meta.AI = parseAIConfig(raw.AI)
		if err := validateAIConfig(meta.AI); err != nil {
			return nil, fmt.Errorf("ai: %w", err)
		}
	}

	meta.Constants = appendSortedEntries(meta.Constants, raw.Constants, resolveEnvRef)
	meta.Secrets = appendSortedEntries(meta.Secrets, raw.Secrets, resolveEnvRef)

	meta.Variables = append(meta.Variables, decodeEntries(&raw.Variables, resolveEnvRef)...)
	meta.SecretVariables = append(meta.SecretVariables, decodeEntries(&raw.SecretVariables, resolveEnvRef)...)

	workflows := make([]*model.Workflow, 0, len(raw.Workflows))
	workflowByID := make(map[string]*model.Workflow, len(raw.Workflows))
	for index, rawWF := range raw.Workflows {
		wf, err := parseWorkflow(rawWF, projectID)
		if err != nil {
			return nil, fmt.Errorf("workflow %d: %w", index+1, err)
		}
		if _, exists := workflowByID[wf.ID]; exists {
			return nil, fmt.Errorf("workflow %d: duplicate id %q", index+1, wf.ID)
		}
		workflows = append(workflows, wf)
		workflowByID[wf.ID] = wf
	}

	return &model.Definition{Meta: meta, Workflows: workflows, WorkflowByID: workflowByID}, nil
}

func parseAIConfig(raw *rawAIConfig) *model.AIConfig {
	if raw == nil {
		return nil
	}

	enabled := true
	if raw.Enabled != nil {
		enabled = *raw.Enabled
	}

	redactSecrets := true
	if raw.RedactSecrets != nil {
		redactSecrets = *raw.RedactSecrets
	}

	return &model.AIConfig{
		Enabled:        enabled,
		Provider:       strings.ToLower(strings.TrimSpace(resolveEnvRef(raw.Provider))),
		BaseURL:        strings.TrimSpace(resolveEnvRef(raw.BaseURL)),
		Model:          strings.TrimSpace(resolveEnvRef(raw.Model)),
		TimeoutSeconds: raw.TimeoutSeconds,
		RedactSecrets:  redactSecrets,
	}
}

func validateAIConfig(config *model.AIConfig) error {
	if config == nil {
		return nil
	}
	if config.TimeoutSeconds < 0 {
		return fmt.Errorf("timeout_seconds must be non-negative")
	}
	if config.Provider != "" && config.Provider != "ollama" && config.Provider != "openai_compatible" {
		return fmt.Errorf("provider must be one of: ollama, openai_compatible")
	}
	if !config.Enabled {
		return nil
	}
	if config.Provider == "" {
		return fmt.Errorf("provider is required when ai is enabled")
	}
	if config.BaseURL == "" {
		return fmt.Errorf("base_url is required when ai is enabled")
	}
	if config.Model == "" {
		return fmt.Errorf("model is required when ai is enabled")
	}
	return nil
}

func parseWorkflow(raw rawWorkflow, projectID string) (*model.Workflow, error) {
	workflowID := strings.TrimSpace(raw.ID)
	if workflowID == "" {
		return nil, fmt.Errorf("id is required")
	}
	wf := &model.Workflow{
		ID:             workflowID,
		ProjectID:      projectID,
		Name:           raw.Name,
		DisableRun:     raw.DisableRun,
		TimeoutSeconds: raw.TimeoutSeconds,
	}
	if wf.Name == "" {
		wf.Name = workflowID
	}

	wf.Constants = appendSortedEntries(wf.Constants, raw.Constants, resolveEnvRef)
	wf.Secrets = appendSortedEntries(wf.Secrets, raw.Secrets, resolveEnvRef)
	if len(raw.Outputs) != 0 {
		wf.Outputs = make(map[string]string, len(raw.Outputs))
		for key, value := range raw.Outputs {
			trimmedKey := strings.TrimSpace(key)
			trimmedValue := strings.TrimSpace(value)
			if trimmedKey == "" || trimmedValue == "" {
				continue
			}
			wf.Outputs[trimmedKey] = trimmedValue
		}
	}

	wf.Variables = append(wf.Variables, decodeEntries(&raw.Variables, resolveEnvRef)...)
	wf.SecretVariables = append(wf.SecretVariables, decodeEntries(&raw.SecretVariables, resolveEnvRef)...)

	steps, err := parseBlocks(raw.Blocks)
	if err != nil {
		return nil, err
	}
	wf.Steps = steps

	errorSteps, err := parseBlocks(raw.CatchErrorBlocks)
	if err != nil {
		return nil, fmt.Errorf("catch_error_blocks: %w", err)
	}
	wf.OnErrorSteps = errorSteps

	return wf, nil
}

func orderedWorkflowIDs(workflows []*model.Workflow) []string {
	ids := make([]string, 0, len(workflows))
	for _, workflow := range workflows {
		if workflow == nil || workflow.ID == "" {
			continue
		}
		ids = append(ids, workflow.ID)
	}
	return ids
}

func parseBlocks(blocks orderedBlocks) ([]model.Step, error) {
	steps := make([]model.Step, 0, len(blocks.Items))
	for _, entry := range blocks.Items {
		step, err := parseBlock(entry.ID, entry.Values)
		if err != nil {
			return nil, fmt.Errorf("block %q: %w", entry.ID, err)
		}
		steps = append(steps, step)
	}
	return steps, nil
}

func parseBlock(blockKey string, raw map[string]any) (model.Step, error) {
	step := model.Step{
		ID:      blockKey,
		BlockID: blockKey,
		Enabled: true,
		Config:  map[string]any{},
	}

	if name, ok := raw["name"].(string); ok && name != "" {
		step.Name = name
	} else {
		step.Name = blockKey
	}

	blockType, ok := raw["type"].(string)
	if !ok || strings.TrimSpace(blockType) == "" {
		return model.Step{}, fmt.Errorf("type is required")
	}
	step.Type = model.BlockType(strings.ToLower(strings.TrimSpace(blockType)))

	if enabled, ok := raw["enabled"].(bool); ok {
		step.Enabled = enabled
	}
	if continueOnError, ok := raw["continue_on_error"].(bool); ok {
		step.ContinueOnError = continueOnError
	}

	for k, v := range raw {
		key := normalizeStepConfigKey(k)
		switch k {
		case "name", "type", "enabled", "continue_on_error":
			continue
		}
		switch key {
		case "assign":
			step.Config["assign"] = mergeAssignConfig(step.Config["assign"], v)
		case "condition":
			condition, err := parseCondition(v)
			if err != nil {
				return model.Step{}, fmt.Errorf("condition: %w", err)
			}
			step.Condition = condition
		case "target_step_id":
			step.Config[key] = strings.TrimSpace(fmt.Sprint(v))
		case "target_workflow_id":
			step.Config[key] = strings.TrimSpace(fmt.Sprint(v))
		default:
			step.Config[key] = v
		}
	}

	return step, nil
}

func parseCondition(raw any) (*model.Condition, error) {
	if raw == nil {
		return nil, nil
	}

	typed, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("must be a mapping")
	}

	condition := &model.Condition{
		Variable: decodeOptionalString(typed, "variable"),
		Operator: decodeOptionalString(typed, "operator"),
		Expected: decodeOptionalString(typed, "expected"),
	}

	if condition.Variable == "" && condition.Operator == "" && condition.Expected == "" {
		return nil, nil
	}

	return condition, nil
}

func decodeOptionalString(values map[string]any, key string) string {
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func mergeAssignConfig(existing any, value any) []any {
	entries := decodeAssignConfig(existing)
	return append(entries, convertAssignShorthand(value)...)
}

func decodeAssignConfig(v any) []any {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	return arr
}

func convertAssignShorthand(v any) []any {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	entries := make([]any, 0, len(m))
	for varName, path := range m {
		entries = append(entries, map[string]any{
			"key":  strings.TrimSpace(varName),
			"path": strings.TrimSpace(fmt.Sprint(path)),
		})
	}
	return entries
}

func resolveEnvRef(v string) string {
	if strings.HasPrefix(v, "${") && strings.HasSuffix(v, "}") {
		inner := v[2 : len(v)-1]
		if idx := strings.Index(inner, ":="); idx != -1 {
			envName := inner[:idx]
			defaultVal := inner[idx+2:]
			if val := os.Getenv(envName); val != "" {
				return val
			}
			return defaultVal
		}
		return os.Getenv(inner)
	}
	return v
}

// decodeEntries accepts either a sequence of strings or a mapping of key/value pairs.
// transform is applied to each value before storing.
func decodeEntries(node *yaml.Node, transform func(string) string) []model.Entry {
	if node == nil || node.Kind == 0 {
		return nil
	}
	if node.Kind == yaml.SequenceNode {
		entries := make([]model.Entry, 0, len(node.Content))
		for _, child := range node.Content {
			if child.Value != "" {
				entries = append(entries, model.Entry{Key: child.Value, Value: ""})
			}
		}
		return entries
	}
	if node.Kind == yaml.MappingNode {
		entries := make([]model.Entry, 0, len(node.Content)/2)
		for i := 0; i+1 < len(node.Content); i += 2 {
			var value any
			if err := node.Content[i+1].Decode(&value); err != nil {
				value = node.Content[i+1].Value
			}
			stringValue := ""
			if value != nil {
				stringValue = fmt.Sprint(value)
			}
			entries = append(entries, model.Entry{
				Key:   node.Content[i].Value,
				Value: transform(stringValue),
			})
		}
		return entries
	}
	return nil
}

func appendSortedEntries(
	dst []model.Entry,
	values map[string]string,
	transform func(string) string,
) []model.Entry {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	for _, key := range keys {
		dst = append(dst, model.Entry{
			Key:   key,
			Value: transform(values[key]),
		})
	}

	return dst
}

func normalizeStepConfigKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}
