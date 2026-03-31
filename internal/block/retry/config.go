package retry

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/wiregoblin/wiregoblin/internal/block"
	"github.com/wiregoblin/wiregoblin/internal/model"
)

type retryConfig struct {
	MaxAttempts int
	DelayMS     int
	RetryOn     retryOnConfig
	Block       model.Step
}

type retryOnConfig struct {
	StatusCodes     []int
	TransportErrors bool
	Enabled         bool
}

func decodeConfig(step model.Step) (retryConfig, error) {
	config := retryConfig{
		MaxAttempts: 1,
	}

	if value, ok := decodeInt(step.Config["max_attempts"]); ok {
		config.MaxAttempts = value
	}
	if value, ok := decodeInt(step.Config["delay_ms"]); ok {
		config.DelayMS = value
	}
	config.RetryOn = decodeRetryOn(step.Config["retry_on"])

	nested, err := block.DecodeInlineStep(step.Config["block"], step.Name+".retry")
	if err != nil {
		return retryConfig{}, err
	}
	config.Block = nested

	return config, nil
}

func decodeRetryOn(raw any) retryOnConfig {
	switch typed := raw.(type) {
	case map[string]any:
		config := retryOnConfig{Enabled: true}
		if value, ok := typed["transport_errors"].(bool); ok {
			config.TransportErrors = value
		}
		config.StatusCodes = decodeIntSlice(typed["status_codes"])
		return config
	default:
		return retryOnConfig{}
	}
}

func decodeIntSlice(raw any) []int {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	result := make([]int, 0, len(items))
	for _, item := range items {
		if value, ok := decodeInt(item); ok {
			result = append(result, value)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func decodeInt(raw any) (int, bool) {
	switch typed := raw.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case string:
		value, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0, false
		}
		return value, true
	default:
		return 0, false
	}
}

func nextDelayMS(baseDelayMS, attempt int) int {
	if baseDelayMS <= 0 || attempt <= 0 {
		return 0
	}

	delay := int64(baseDelayMS)
	for i := 1; i < attempt; i++ {
		delay *= 2
	}
	if delay > int64(^uint(0)>>1) {
		return int(^uint(0) >> 1)
	}
	return int(delay)
}

func waitForDelay(ctx context.Context, delayMS int) error {
	if delayMS <= 0 {
		return nil
	}

	timer := time.NewTimer(time.Duration(delayMS) * time.Millisecond)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
