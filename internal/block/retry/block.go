// Package retry implements retry workflow steps with doubling backoff.
package retry

import (
	"context"
	"fmt"
	"strconv"

	"github.com/wiregoblin/wiregoblin/internal/block"
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
	if config.RetryOn.Enabled && len(config.RetryOn.StatusCodes) == 0 && !config.RetryOn.TransportErrors {
		return fmt.Errorf("retry retry_on must define status_codes or transport_errors")
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

		retryable := shouldRetry(config.RetryOn, result, execErr)
		lastRetryable = retryable
		if !retryable {
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
	if err == nil {
		return false
	}
	if !config.Enabled {
		return true
	}
	if statusCode, ok := extractStatusCode(result); ok {
		return containsStatusCode(config.StatusCodes, statusCode)
	}
	return config.TransportErrors
}

func extractStatusCode(result *block.Result) (int, bool) {
	if result == nil {
		return 0, false
	}
	if value, ok := result.Exports["statusCode"]; ok {
		statusCode, err := strconv.Atoi(value)
		if err == nil {
			return statusCode, true
		}
	}
	output, ok := result.Output.(map[string]any)
	if !ok {
		return 0, false
	}
	raw, exists := output["statusCode"]
	if !exists {
		return 0, false
	}
	statusCode, ok := decodeInt(raw)
	return statusCode, ok
}

func containsStatusCode(codes []int, target int) bool {
	for _, code := range codes {
		if code == target {
			return true
		}
	}
	return false
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
