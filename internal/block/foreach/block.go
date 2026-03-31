// Package foreach implements foreach workflow steps over JSON arrays.
package foreach

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/model"
)

const blockType = "foreach"

// Block iterates an embedded block over a list of items.
type Block struct{}

// New creates a foreach workflow block.
func New() *Block {
	return &Block{}
}

// Type returns the foreach block type identifier.
func (b *Block) Type() model.BlockType {
	return blockType
}

// SupportsResponseMapping reports whether foreach output can be assigned into runtime variables.
func (b *Block) SupportsResponseMapping() bool {
	return true
}

// ReferencePolicy describes which foreach fields accept constants and runtime variables.
func (b *Block) ReferencePolicy() []block.ReferencePolicy {
	return []block.ReferencePolicy{
		{Field: "items", Constants: true, Variables: true, InlineOnly: true},
	}
}

// Validate checks the foreach configuration.
func (b *Block) Validate(step model.Step) error {
	config, err := decodeConfig(step)
	if err != nil {
		return err
	}
	if len(config.Items) == 0 {
		return fmt.Errorf("foreach items must contain at least one element")
	}
	if config.Concurrency < 0 {
		return fmt.Errorf("foreach concurrency must be greater than 0")
	}
	if config.Concurrency > 1 && block.HasNestedAssignments(config.Block.Config) {
		return fmt.Errorf(
			"foreach concurrency > 1 does not support nested runtime assignments; use collect instead",
		)
	}
	for target := range config.Collect {
		if !strings.HasPrefix(strings.TrimSpace(target), "$") {
			return fmt.Errorf("foreach collect target must start with $")
		}
	}
	return nil
}

// Execute runs one nested block for each item in the configured array.
func (b *Block) Execute(ctx context.Context, runCtx *block.RunContext, step model.Step) (*block.Result, error) {
	config, err := decodeConfig(step)
	if err != nil {
		return nil, err
	}
	if runCtx == nil || runCtx.ExecuteStep == nil {
		return nil, fmt.Errorf("foreach block requires step execution support")
	}

	concurrency := config.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}

	if concurrency == 1 {
		return executeSequential(ctx, runCtx, config)
	}
	if runCtx.ExecuteIsolatedStep == nil {
		return nil, fmt.Errorf("foreach concurrency > 1 requires isolated step execution support")
	}
	return executeConcurrent(ctx, runCtx, config, concurrency)
}

func executeSequential(
	ctx context.Context,
	runCtx *block.RunContext,
	config foreachConfig,
) (*block.Result, error) {
	previousBuiltins := block.CloneStringMap(runCtx.Builtins)
	results := make([]map[string]any, 0, len(config.Items))
	total := len(config.Items)
	collected := make(map[string][]any, len(config.Collect))

	for index, item := range config.Items {
		setLoopBuiltins(runCtx, item, index, total)
		result, execErr := runCtx.ExecuteStep(ctx, config.Block)

		itemResult := buildItemResult(index, item, result, execErr)
		appendCollected(collected, config.Collect, itemResult)
		results = append(results, itemResult)
		if execErr != nil {
			restoreBuiltins(runCtx, previousBuiltins)
			output, exports := foreachOutput(total, results, collected)
			return &block.Result{Output: output, Exports: exports}, execErr
		}
	}

	restoreBuiltins(runCtx, previousBuiltins)
	output, exports := foreachOutput(total, results, collected)
	return &block.Result{Output: output, Exports: exports}, nil
}

func executeConcurrent(
	ctx context.Context,
	runCtx *block.RunContext,
	config foreachConfig,
	concurrency int,
) (*block.Result, error) {
	type task struct {
		index int
		item  any
	}
	type outcome struct {
		index  int
		result map[string]any
		err    error
	}

	total := len(config.Items)
	tasks := make(chan task)
	outcomes := make(chan outcome, total)

	var wg sync.WaitGroup
	workerCount := min(total, concurrency)
	for range workerCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range tasks {
				isolated := block.CloneRunContext(runCtx)
				setLoopBuiltins(isolated, task.item, task.index, total)
				result, execErr := runCtx.ExecuteIsolatedStep(ctx, isolated, config.Block)
				outcomes <- outcome{
					index:  task.index,
					result: buildItemResult(task.index, task.item, result, execErr),
					err:    execErr,
				}
			}
		}()
	}

	go func() {
		defer close(tasks)
		for index, item := range config.Items {
			select {
			case <-ctx.Done():
				return
			case tasks <- task{index: index, item: item}:
			}
		}
	}()

	go func() {
		wg.Wait()
		close(outcomes)
	}()

	ordered := make([]map[string]any, total)
	failed := 0
	var firstErr error
	for outcome := range outcomes {
		ordered[outcome.index] = outcome.result
		if outcome.err != nil {
			failed++
			if firstErr == nil {
				firstErr = outcome.err
			}
		}
	}

	collected := make(map[string][]any, len(config.Collect))
	for _, itemResult := range ordered {
		appendCollected(collected, config.Collect, itemResult)
	}

	output, exports := foreachOutput(total, ordered, collected)
	if failed == 0 {
		return &block.Result{Output: output, Exports: exports}, nil
	}

	return &block.Result{Output: output, Exports: exports}, fmt.Errorf(
		"foreach failed: %d of %d iterations failed: %w",
		failed,
		total,
		firstErr,
	)
}

func buildItemResult(index int, item any, result *block.Result, execErr error) map[string]any {
	itemResult := map[string]any{
		"index": index,
		"item":  item,
	}
	if result != nil {
		itemResult["output"] = result.Output
		if len(result.Exports) != 0 {
			itemResult["exports"] = result.Exports
		}
	}
	if execErr != nil {
		itemResult["error"] = execErr.Error()
	}
	return itemResult
}

func foreachOutput(
	total int,
	results []map[string]any,
	collected map[string][]any,
) (map[string]any, map[string]string) {
	output := map[string]any{
		"count":   total,
		"results": results,
	}
	if len(collected) == 0 {
		return output, nil
	}
	collectedAny := make(map[string]any, len(collected))
	exports := make(map[string]string, len(collected))
	for key, values := range collected {
		name := block.CollectedName(key)
		collectedAny[name] = values
		body, err := json.Marshal(values)
		if err == nil {
			exports[key] = string(body)
		}
	}
	output["collected"] = collectedAny
	return output, exports
}

func appendCollected(collected map[string][]any, collect map[string]string, itemResult map[string]any) {
	if len(collect) == 0 {
		return
	}
	for key, path := range collect {
		value, ok := block.ReadCollectedValue(itemResult, path)
		if !ok {
			value = nil
		}
		collected[key] = append(collected[key], value)
	}
}

func setLoopBuiltins(runCtx *block.RunContext, item any, index, total int) {
	if runCtx == nil {
		return
	}
	runCtx.Builtins["Each.Index"] = fmt.Sprintf("%d", index)
	runCtx.Builtins["Each.Count"] = fmt.Sprintf("%d", total)
	runCtx.Builtins["Each.First"] = fmt.Sprintf("%t", index == 0)
	runCtx.Builtins["Each.Last"] = fmt.Sprintf("%t", index == total-1)
	setItemBuiltins(runCtx, "Each.Item", item)
}

func setItemBuiltins(runCtx *block.RunContext, prefix string, value any) {
	if runCtx == nil {
		return
	}
	if encoded, err := json.Marshal(value); err == nil {
		runCtx.Builtins[prefix+"JSON"] = string(encoded)
	}
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			setItemBuiltins(runCtx, prefix+"."+key, item)
		}
	case []any:
		if encoded, err := json.Marshal(typed); err == nil {
			runCtx.Builtins[prefix] = string(encoded)
		}
	case string:
		runCtx.Builtins[prefix] = typed
	case bool, int, int64, float64:
		runCtx.Builtins[prefix] = fmt.Sprint(typed)
	default:
		if value != nil {
			runCtx.Builtins[prefix] = fmt.Sprint(value)
		}
	}
}

func restoreBuiltins(runCtx *block.RunContext, snapshot map[string]string) {
	if runCtx == nil {
		return
	}
	runCtx.Builtins = block.CloneStringMap(snapshot)
}
