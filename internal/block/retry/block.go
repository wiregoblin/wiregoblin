// Package retry implements retry workflow steps with doubling backoff.
package retry

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/condition"
	"github.com/wiregoblin/wiregoblin/internal/model"
)

const blockType = "retry"

// Block retries an embedded block with exponential backoff.
type Block struct{}

// New creates a retry workflow block.
func New() *Block {
	return &Block{}
}

// Type returns the retry block type identifier.
func (b *Block) Type() model.BlockType {
	return blockType
}

// Validate checks the retry configuration.
func (b *Block) Validate(step model.Step) error {
	config, err := decodeConfig(step)
	if err != nil {
		return err
	}
	if config.MaxAttempts <= 0 {
		return fmt.Errorf("retry max_attempts must be greater than 0")
	}
	if config.DelayMS < 0 {
		return fmt.Errorf("retry delay_ms must be non-negative")
	}
	if err := config.RetryOn.Validate(); err != nil {
		return err
	}
	if config.Block.Type == blockType {
		return fmt.Errorf("retry block cannot wrap another retry block directly")
	}
	return nil
}

// Execute retries one nested block until it succeeds or the attempt budget is exhausted.
func (b *Block) Execute(ctx context.Context, runCtx *block.RunContext, step model.Step) (*block.Result, error) {
	config, err := decodeConfig(step)
	if err != nil {
		return nil, err
	}
	if runCtx == nil || runCtx.ExecuteStep == nil {
		return nil, fmt.Errorf("retry block requires step execution support")
	}

	var lastResult *block.Result
	var lastErr error
	var lastRetryable bool
	var stoppedEarly bool
	lastAttempt := 0

	for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
		lastAttempt = attempt
		setAttemptBuiltins(runCtx, attempt, config.MaxAttempts)
		result, execErr := runCtx.ExecuteStep(ctx, config.Block)
		lastResult = result
		lastErr = execErr
		retryable := shouldRetry(config.RetryOn, result, execErr)
		lastRetryable = retryable
		if !retryable {
			if execErr == nil {
				clearAttemptBuiltins(runCtx)
				return &block.Result{
					Output: map[string]any{
						"attempts":     attempt,
						"max_attempts": config.MaxAttempts,
						"delay_ms":     config.DelayMS,
						"succeeded":    true,
						"retryable":    false,
						"result":       outputOrNil(result),
					},
				}, nil
			}
			stoppedEarly = config.RetryOn.Enabled
			break
		}

		if attempt == config.MaxAttempts {
			break
		}
		if err := waitForRetry(ctx, config.DelayMS, attempt); err != nil {
			clearAttemptBuiltins(runCtx)
			return &block.Result{
				Output: map[string]any{
					"attempts":      attempt,
					"max_attempts":  config.MaxAttempts,
					"delay_ms":      config.DelayMS,
					"succeeded":     false,
					"retryable":     retryable,
					"stopped_early": stoppedEarly,
					"result":        outputOrNil(lastResult),
					"last_error":    execErr.Error(),
				},
			}, err
		}
	}

	clearAttemptBuiltins(runCtx)
	return &block.Result{
		Output: map[string]any{
			"attempts":      lastAttempt,
			"max_attempts":  config.MaxAttempts,
			"delay_ms":      config.DelayMS,
			"succeeded":     false,
			"retryable":     lastRetryable,
			"stopped_early": stoppedEarly,
			"result":        outputOrNil(lastResult),
			"last_error":    errorString(lastErr),
		},
	}, lastErr
}

func shouldRetry(config retryOnConfig, result *block.Result, err error) bool {
	if !config.Enabled {
		return err != nil
	}
	matches := make([]bool, 0, len(config.Rules))
	for _, rule := range config.Rules {
		matches = append(matches, matchesRetryRule(rule, result, err))
	}
	if config.Match == "all" {
		for _, match := range matches {
			if !match {
				return false
			}
		}
		return len(matches) != 0
	}
	for _, match := range matches {
		if match {
			return true
		}
	}
	return false
}

func matchesRetryRule(rule retryRule, result *block.Result, err error) bool {
	switch rule.Type {
	case "transport_error":
		return err != nil && extractStatusCode(result) == nil
	case "status_code":
		statusCode := extractStatusCode(result)
		if statusCode == nil {
			return false
		}
		return containsStatusCode(rule.In, *statusCode)
	case "path":
		value, ok := readRetryPath(result, rule.Path)
		if !ok {
			return false
		}
		return evaluatePathRule(value, rule.Operator, rule.Expected)
	default:
		return false
	}
}

func extractStatusCode(result *block.Result) *int {
	if result == nil {
		return nil
	}
	if value, ok := result.Exports["statusCode"]; ok {
		statusCode, err := strconv.Atoi(value)
		if err == nil {
			return &statusCode
		}
	}
	output, ok := result.Output.(map[string]any)
	if !ok {
		return nil
	}
	raw, exists := output["statusCode"]
	if !exists {
		return nil
	}
	statusCode, ok := decodeInt(raw)
	if !ok {
		return nil
	}
	return &statusCode
}

func containsStatusCode(codes []int, target int) bool {
	for _, code := range codes {
		if code == target {
			return true
		}
	}
	return false
}

func readRetryPath(result *block.Result, path string) (any, bool) {
	path = strings.TrimSpace(path)
	if result == nil || path == "" {
		return nil, false
	}
	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return nil, false
	}

	var current any
	switch parts[0] {
	case "body":
		current, _ = readRetryBody(result)
		parts = parts[1:]
	case "statusCode":
		statusCode := extractStatusCode(result)
		if statusCode == nil {
			return nil, false
		}
		current = *statusCode
		parts = parts[1:]
	default:
		current = result.Output
	}
	if len(parts) == 0 {
		return current, current != nil
	}

	for _, part := range parts {
		value, ok := readRetryPart(current, part)
		if !ok {
			return nil, false
		}
		current = value
	}
	return current, true
}

func readRetryBody(result *block.Result) (any, bool) {
	if result == nil {
		return nil, false
	}
	outputMap, ok := result.Output.(map[string]any)
	if !ok {
		return result.Output, result.Output != nil
	}
	body, found := outputMap["body"]
	return body, found
}

func readRetryPart(current any, part string) (any, bool) {
	switch typed := current.(type) {
	case map[string]any:
		if strings.EqualFold(part, "length") {
			return len(typed), true
		}
		return resolveRetryKey(typed, part)
	case []any:
		if strings.EqualFold(part, "length") {
			return len(typed), true
		}
		index, err := strconv.Atoi(part)
		if err != nil || index < 0 || index >= len(typed) {
			return nil, false
		}
		return typed[index], true
	case string:
		if strings.EqualFold(part, "length") {
			return len(typed), true
		}
		return nil, false
	default:
		return nil, false
	}
}

func resolveRetryKey(m map[string]any, key string) (any, bool) {
	if value, ok := m[key]; ok {
		return value, true
	}
	for candidate, value := range m {
		if strings.EqualFold(candidate, key) {
			return value, true
		}
	}
	return nil, false
}

func evaluatePathRule(actual any, operator string, expected any) bool {
	switch strings.ToLower(strings.TrimSpace(operator)) {
	case "empty":
		return isEmptyValue(actual)
	case "not_empty":
		return !isEmptyValue(actual)
	default:
		return compareRetryValues(actual, operator, expected)
	}
}

func isEmptyValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(typed) == ""
	case []any:
		return len(typed) == 0
	case map[string]any:
		return len(typed) == 0
	default:
		return false
	}
}

func compareRetryValues(actual any, operator string, expected any) bool {
	if left, ok := normalizeRetryNumber(actual); ok {
		if right, ok := normalizeRetryNumber(expected); ok {
			return compareRetryNumbers(left, operator, right)
		}
	}
	matched, err := condition.Evaluate(fmt.Sprint(actual), operator, fmt.Sprint(expected))
	return err == nil && matched
}

func normalizeRetryNumber(value any) (float64, bool) {
	switch typed := value.(type) {
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func compareRetryNumbers(actual float64, operator string, expected float64) bool {
	switch strings.TrimSpace(operator) {
	case "=":
		return actual == expected
	case "!=":
		return actual != expected
	case ">":
		return actual > expected
	case "<":
		return actual < expected
	case ">=":
		return actual >= expected
	case "<=":
		return actual <= expected
	default:
		return false
	}
}

func outputOrNil(result *block.Result) any {
	if result == nil {
		return nil
	}
	return result.Output
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func setAttemptBuiltins(runCtx *block.RunContext, attempt, maxAttempts int) {
	if runCtx == nil {
		return
	}
	runCtx.Builtins["Retry.Attempt"] = fmt.Sprintf("%d", attempt)
	runCtx.Builtins["Retry.MaxAttempts"] = fmt.Sprintf("%d", maxAttempts)
}

func clearAttemptBuiltins(runCtx *block.RunContext) {
	if runCtx == nil {
		return
	}
	delete(runCtx.Builtins, "Retry.Attempt")
	delete(runCtx.Builtins, "Retry.MaxAttempts")
}

func waitForRetry(ctx context.Context, baseDelayMS, attempt int) error {
	return waitForDelay(ctx, nextDelayMS(baseDelayMS, attempt))
}
