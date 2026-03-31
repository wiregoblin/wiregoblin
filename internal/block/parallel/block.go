// Package parallel implements concurrent execution of heterogeneous nested blocks.
package parallel

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/model"
)

const blockType = "parallel"

// Block executes multiple nested blocks concurrently.
type Block struct{}

// New creates a parallel workflow block.
func New() *Block {
	return &Block{}
}

// Type returns the block type identifier.
func (b *Block) Type() model.BlockType {
	return blockType
}

// SupportsResponseMapping reports whether parallel output can be assigned into runtime variables.
func (b *Block) SupportsResponseMapping() bool {
	return true
}

// Validate checks the parallel configuration.
func (b *Block) Validate(step model.Step) error {
	config, err := decodeConfig(step)
	if err != nil {
		return err
	}
	if len(config.Blocks) == 0 {
		return fmt.Errorf("parallel blocks must contain at least one block")
	}
	for _, nested := range config.Blocks {
		if block.HasNestedAssignments(nested.Config) {
			return fmt.Errorf("parallel blocks do not support nested runtime assignments; use collect instead")
		}
	}
	for target := range config.Collect {
		if !strings.HasPrefix(strings.TrimSpace(target), "$") {
			return fmt.Errorf("parallel collect target must start with $")
		}
	}
	return nil
}

// Execute runs all nested blocks concurrently and aggregates their outputs.
func (b *Block) Execute(ctx context.Context, runCtx *block.RunContext, step model.Step) (*block.Result, error) {
	config, err := decodeConfig(step)
	if err != nil {
		return nil, err
	}
	if runCtx == nil || runCtx.ExecuteIsolatedStep == nil {
		return nil, fmt.Errorf("parallel block requires isolated step execution support")
	}

	type outcome struct {
		id     string
		result map[string]any
		err    error
	}

	outcomes := make(chan outcome, len(config.Blocks))
	var wg sync.WaitGroup
	for _, nested := range config.Blocks {
		nested := nested
		wg.Add(1)
		go func() {
			defer wg.Done()
			isolated := block.CloneRunContext(runCtx)
			result, execErr := runCtx.ExecuteIsolatedStep(ctx, isolated, nested)
			outcomes <- outcome{
				id:     nested.ID,
				result: buildBranchResult(result, execErr),
				err:    execErr,
			}
		}()
	}

	go func() {
		wg.Wait()
		close(outcomes)
	}()

	results := make(map[string]any, len(config.Blocks))
	failed := 0
	var firstErr error
	for outcome := range outcomes {
		results[outcome.id] = outcome.result
		if outcome.err != nil {
			failed++
			if firstErr == nil {
				firstErr = outcome.err
			}
		}
	}

	collected, exports := collectResults(config.Collect, results)
	output := map[string]any{
		"results": results,
	}
	if len(collected) != 0 {
		output["collected"] = collected
	}

	if failed == 0 {
		return &block.Result{Output: output, Exports: exports}, nil
	}
	return &block.Result{Output: output, Exports: exports}, fmt.Errorf(
		"parallel failed: %d of %d branches failed: %w",
		failed,
		len(config.Blocks),
		firstErr,
	)
}

func buildBranchResult(result *block.Result, execErr error) map[string]any {
	item := map[string]any{}
	if result != nil {
		item["output"] = result.Output
		if len(result.Exports) != 0 {
			item["exports"] = result.Exports
		}
	}
	if execErr != nil {
		item["error"] = execErr.Error()
	}
	return item
}

func collectResults(collect map[string]string, results map[string]any) (map[string]any, map[string]string) {
	if len(collect) == 0 {
		return nil, nil
	}
	collected := make(map[string]any, len(collect))
	exports := make(map[string]string, len(collect))
	for target, path := range collect {
		value, ok := block.ReadCollectedValue(results, path)
		if !ok {
			value = nil
		}
		name := block.CollectedName(target)
		collected[name] = value
		if serialized, ok := stringifyCollectedValue(value); ok {
			exports[target] = serialized
		}
	}
	return collected, exports
}

func stringifyCollectedValue(value any) (string, bool) {
	switch typed := value.(type) {
	case nil:
		return "", false
	case string:
		return typed, true
	case bool, int, int64, float64:
		return fmt.Sprint(typed), true
	default:
		body, err := json.Marshal(value)
		if err != nil {
			return "", false
		}
		return string(body), true
	}
}
