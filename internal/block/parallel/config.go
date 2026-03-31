package parallel

import (
	"fmt"
	"strings"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/model"
)

type parallelConfig struct {
	Blocks  []model.Step
	Collect map[string]string
}

func decodeConfig(step model.Step) (parallelConfig, error) {
	blocks, err := decodeBlocks(step.Config["blocks"], step.Name+".parallel")
	if err != nil {
		return parallelConfig{}, err
	}

	return parallelConfig{
		Blocks:  blocks,
		Collect: block.DecodeCollect(step.Config["collect"]),
	}, nil
}

func decodeBlocks(raw any, defaultPrefix string) ([]model.Step, error) {
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("parallel blocks must be an array")
	}
	steps := make([]model.Step, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for index, item := range items {
		configMap, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("parallel block %d must be an object", index+1)
		}
		id := strings.TrimSpace(fmt.Sprint(configMap["id"]))
		if id == "" {
			return nil, fmt.Errorf("parallel block %d id is required", index+1)
		}
		if _, exists := seen[id]; exists {
			return nil, fmt.Errorf("parallel block id %q must be unique", id)
		}
		seen[id] = struct{}{}

		step, err := block.DecodeInlineStep(configMap, defaultPrefix+"."+id)
		if err != nil {
			return nil, fmt.Errorf("parallel block %q: %w", id, err)
		}
		step.ID = id
		step.BlockID = id
		if step.Name == defaultPrefix+"."+id {
			step.Name = id
		}
		steps = append(steps, step)
	}
	return steps, nil
}
