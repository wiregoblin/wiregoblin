// Package filerepository loads project definitions from YAML files on disk.
package filerepository

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"

	"github.com/wiregoblin/wiregoblin/internal/models"
)

// Repository loads project definitions from one YAML file on disk.
type Repository struct {
	path string
}

// New creates a file-backed project repository.
func New(path string) *Repository {
	return &Repository{path: path}
}

// GetProject loads and parses one project definition from disk.
func (r *Repository) GetProject() (*models.Definition, error) {
	data, err := os.ReadFile(r.path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", r.path, err)
	}
	return parse(data)
}

type rawConfig struct {
	ID        string                 `yaml:"id"`
	Name      string                 `yaml:"name"`
	Version   int                    `yaml:"version"`
	Constants map[string]string      `yaml:"constants"`
	Secrets   map[string]string      `yaml:"secrets"`
	Variables yaml.Node              `yaml:"variables"`
	Workflows map[string]rawWorkflow `yaml:"workflows"`
}

type rawWorkflow struct {
	Name             string            `yaml:"name"`
	Constants        map[string]string `yaml:"constants"`
	Variables        yaml.Node         `yaml:"variables"`
	Blocks           orderedMap        `yaml:"blocks"`
	CatchErrorBlocks orderedMap        `yaml:"catchErrorBlocks"`
}

// orderedMap preserves YAML mapping key order during unmarshalling.
type orderedMap struct {
	Keys   []string
	Values map[string]map[string]any
}

func (m *orderedMap) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode && value.Value == "" {
		return nil
	}
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("expected a mapping for blocks, got %v", value.Kind)
	}
	m.Values = make(map[string]map[string]any)
	for i := 0; i+1 < len(value.Content); i += 2 {
		key := value.Content[i].Value
		var val map[string]any
		if err := value.Content[i+1].Decode(&val); err != nil {
			return fmt.Errorf("decode block %q: %w", key, err)
		}
		m.Keys = append(m.Keys, key)
		m.Values[key] = val
	}
	return nil
}

func parse(data []byte) (*models.Definition, error) {
	var raw rawConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}

	projectID := nameToUUID("project:" + raw.ID)

	meta := &models.Project{
		ID:   projectID,
		Name: raw.Name,
	}

	meta.Constants = appendSortedEntries(meta.Constants, raw.Constants, func(value string) string {
		return value
	})
	meta.Secrets = appendSortedEntries(meta.Secrets, raw.Secrets, resolveEnvRef)

	meta.Variables = append(meta.Variables, decodeEntries(&raw.Variables)...)

	workflows := make(map[string]*models.Workflow, len(raw.Workflows))
	for wfKey, rawWF := range raw.Workflows {
		wf, err := parseWorkflow(wfKey, rawWF, projectID)
		if err != nil {
			return nil, fmt.Errorf("workflow %q: %w", wfKey, err)
		}
		workflows[wfKey] = wf
	}

	return &models.Definition{Meta: meta, Workflows: workflows}, nil
}

func parseWorkflow(key string, raw rawWorkflow, projectID uuid.UUID) (*models.Workflow, error) {
	wf := &models.Workflow{
		ID:        nameToUUID("workflow:" + key),
		ProjectID: projectID,
		Name:      raw.Name,
	}
	if wf.Name == "" {
		wf.Name = key
	}

	wf.Constants = appendSortedEntries(wf.Constants, raw.Constants, func(value string) string {
		return value
	})

	wf.Variables = append(wf.Variables, decodeEntries(&raw.Variables)...)

	steps, err := parseBlocks(key, raw.Blocks)
	if err != nil {
		return nil, err
	}
	wf.Steps = steps

	errorSteps, err := parseBlocks(key+":onError", raw.CatchErrorBlocks)
	if err != nil {
		return nil, fmt.Errorf("catchErrorBlocks: %w", err)
	}
	wf.OnErrorSteps = errorSteps

	return wf, nil
}

func parseBlocks(workflowKey string, om orderedMap) ([]models.Step, error) {
	steps := make([]models.Step, 0, len(om.Keys))
	for _, blockKey := range om.Keys {
		raw := om.Values[blockKey]
		step, err := parseBlock(workflowKey, blockKey, raw)
		if err != nil {
			return nil, fmt.Errorf("block %q: %w", blockKey, err)
		}
		steps = append(steps, step)
	}
	return steps, nil
}

func parseBlock(workflowKey, blockKey string, raw map[string]any) (models.Step, error) {
	step := models.Step{
		ID:      nameToUUID(workflowKey + ":" + blockKey),
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
		return models.Step{}, fmt.Errorf("type is required")
	}
	step.Type = strings.ToLower(strings.TrimSpace(blockType))

	if enabled, ok := raw["enabled"].(bool); ok {
		step.Enabled = enabled
	}

	for k, v := range raw {
		key := normalizeStepConfigKey(k)
		switch k {
		case "name", "type", "enabled":
			continue
		}
		switch key {
		case "response":
			step.Config["response_mapping"] = convertResponseShorthand(v)
		case "target_step_uid":
			if s, ok := v.(string); ok {
				if _, err := uuid.Parse(s); err != nil {
					v = nameToUUID(workflowKey + ":" + s).String()
				}
			}
			step.Config[key] = v
		case "target_workflow_uid":
			if s, ok := v.(string); ok {
				if _, err := uuid.Parse(s); err != nil {
					v = nameToUUID("workflow:" + s).String()
				}
			}
			step.Config[key] = v
		default:
			step.Config[key] = v
		}
	}

	return step, nil
}

func convertResponseShorthand(v any) []any {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	entries := make([]any, 0, len(m))
	for path, varRef := range m {
		varName := fmt.Sprint(varRef)
		varName = strings.TrimPrefix(varName, "@")
		entries = append(entries, map[string]any{
			"key":  varName,
			"path": path,
		})
	}
	return entries
}

func resolveEnvRef(v string) string {
	if strings.HasPrefix(v, "${") && strings.HasSuffix(v, "}") {
		envName := v[2 : len(v)-1]
		return os.Getenv(envName)
	}
	return v
}

// decodeEntries accepts either a sequence of strings or a mapping of key/value pairs.
func decodeEntries(node *yaml.Node) []models.Entry {
	if node == nil || node.Kind == 0 {
		return nil
	}
	if node.Kind == yaml.SequenceNode {
		entries := make([]models.Entry, 0, len(node.Content))
		for _, child := range node.Content {
			if child.Value != "" {
				entries = append(entries, models.Entry{Key: child.Value, Value: ""})
			}
		}
		return entries
	}
	if node.Kind == yaml.MappingNode {
		entries := make([]models.Entry, 0, len(node.Content)/2)
		for i := 0; i+1 < len(node.Content); i += 2 {
			var value any
			if err := node.Content[i+1].Decode(&value); err != nil {
				value = node.Content[i+1].Value
			}
			stringValue := ""
			if value != nil {
				stringValue = fmt.Sprint(value)
			}
			entries = append(entries, models.Entry{
				Key:   node.Content[i].Value,
				Value: stringValue,
			})
		}
		return entries
	}
	return nil
}

func appendSortedEntries(
	dst []models.Entry,
	values map[string]string,
	transform func(string) string,
) []models.Entry {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	for _, key := range keys {
		dst = append(dst, models.Entry{
			Key:   key,
			Value: transform(values[key]),
		})
	}

	return dst
}

// nameToUUID derives a deterministic UUID v5 from a human-readable name.
// This ensures that goto target_step_uid references stay stable across loads.
var uuidNamespace = uuid.MustParse("6ba7b814-9dad-11d1-80b4-00c04fd430c8")

func nameToUUID(name string) uuid.UUID {
	return uuid.NewSHA1(uuidNamespace, []byte(name))
}

func normalizeStepConfigKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}
