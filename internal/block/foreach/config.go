package foreach

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/model"
)

type foreachConfig struct {
	Items       []any
	Block       model.Step
	Collect     map[string]string
	Concurrency int
}

func decodeConfig(step model.Step) (foreachConfig, error) {
	nested, err := block.DecodeInlineStep(step.Config["block"], step.Name+".foreach")
	if err != nil {
		return foreachConfig{}, err
	}

	items, err := decodeItems(step.Config["items"])
	if err != nil {
		return foreachConfig{}, err
	}

	return foreachConfig{
		Items:       items,
		Block:       nested,
		Collect:     block.DecodeCollect(step.Config["collect"]),
		Concurrency: decodeConcurrency(step.Config["concurrency"]),
	}, nil
}

func decodeConcurrency(raw any) int {
	switch typed := raw.(type) {
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

func decodeItems(raw any) ([]any, error) {
	switch typed := raw.(type) {
	case nil:
		return nil, fmt.Errorf("foreach items are required")
	case []any:
		return typed, nil
	case map[string]any:
		return decodeRangeItems(typed)
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return nil, fmt.Errorf("foreach items are required")
		}
		var items []any
		if err := json.Unmarshal([]byte(trimmed), &items); err != nil {
			return nil, fmt.Errorf("foreach items must be an array or JSON array string")
		}
		return items, nil
	default:
		return nil, fmt.Errorf("foreach items must be an array, JSON array string, or numeric range")
	}
}

func decodeRangeItems(raw map[string]any) ([]any, error) {
	from, err := decodeRangeInt(raw["from"], "from")
	if err != nil {
		return nil, err
	}
	to, err := decodeRangeInt(raw["to"], "to")
	if err != nil {
		return nil, err
	}
	step := 1
	if _, ok := raw["step"]; ok {
		step, err = decodeRangeInt(raw["step"], "step")
		if err != nil {
			return nil, err
		}
	}
	if step == 0 {
		return nil, fmt.Errorf("foreach items range step must not be 0")
	}
	if from < to && step < 0 {
		return nil, fmt.Errorf("foreach items range step must be positive when from < to")
	}
	if from > to && step > 0 {
		return nil, fmt.Errorf("foreach items range step must be negative when from > to")
	}

	items := make([]any, 0, estimateRangeSize(from, to, step))
	if step > 0 {
		for current := from; current <= to; current += step {
			items = append(items, current)
		}
	} else {
		for current := from; current >= to; current += step {
			items = append(items, current)
		}
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("foreach items range must contain at least one element")
	}
	return items, nil
}

func decodeRangeInt(raw any, field string) (int, error) {
	switch typed := raw.(type) {
	case int:
		return typed, nil
	case int64:
		return int(typed), nil
	case float64:
		if math.Trunc(typed) != typed {
			return 0, fmt.Errorf("foreach items range %s must be an integer", field)
		}
		return int(typed), nil
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return 0, fmt.Errorf("foreach items range %s is required", field)
		}
		value, err := strconv.Atoi(trimmed)
		if err != nil {
			return 0, fmt.Errorf("foreach items range %s must be an integer", field)
		}
		return value, nil
	case nil:
		return 0, fmt.Errorf("foreach items range %s is required", field)
	default:
		return 0, fmt.Errorf("foreach items range %s must be an integer", field)
	}
}

func estimateRangeSize(from, to, step int) int {
	distance := to - from
	if distance < 0 {
		distance = -distance
	}
	stepAbs := step
	if stepAbs < 0 {
		stepAbs = -stepAbs
	}
	return distance/stepAbs + 1
}
