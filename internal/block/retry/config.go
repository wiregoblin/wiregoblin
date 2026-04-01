package retry

import (
	"context"
	"fmt"
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
	Match   string
	Rules   []retryRule
	Enabled bool
}

type retryRule struct {
	Type     string
	Path     string
	Operator string
	Expected any
	In       []int
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
	typed, ok := raw.(map[string]any)
	if !ok {
		return retryOnConfig{}
	}
	config := retryOnConfig{
		Match:   "any",
		Enabled: true,
	}
	if value, ok := typed["match"].(string); ok {
		config.Match = strings.ToLower(strings.TrimSpace(value))
	}
	config.Rules = decodeRetryRules(typed["rules"])
	return config
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

func decodeRetryRules(raw any) []retryRule {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	rules := make([]retryRule, 0, len(items))
	for _, item := range items {
		typed, ok := item.(map[string]any)
		if !ok {
			continue
		}
		rule := retryRule{}
		if value, ok := typed["type"].(string); ok {
			rule.Type = strings.ToLower(strings.TrimSpace(value))
		}
		if value, ok := typed["path"].(string); ok {
			rule.Path = strings.TrimSpace(value)
		}
		if value, ok := typed["operator"].(string); ok {
			rule.Operator = strings.TrimSpace(value)
		}
		rule.Expected = typed["expected"]
		rule.In = decodeIntSlice(typed["in"])
		if rule.Type != "" {
			rules = append(rules, rule)
		}
	}
	if len(rules) == 0 {
		return nil
	}
	return rules
}

func (c retryOnConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.Match != "any" && c.Match != "all" {
		return fmt.Errorf("retry retry_on match must be any or all")
	}
	if len(c.Rules) == 0 {
		return fmt.Errorf("retry retry_on must define at least one rule")
	}
	for index, rule := range c.Rules {
		if err := rule.Validate(); err != nil {
			return fmt.Errorf("retry retry_on rule %d: %w", index+1, err)
		}
	}
	return nil
}

func (r retryRule) Validate() error {
	switch r.Type {
	case "transport_error":
		return nil
	case "status_code":
		if len(r.In) == 0 {
			return fmt.Errorf("status_code rule must define in")
		}
		return nil
	case "path":
		if r.Path == "" {
			return fmt.Errorf("path rule path is required")
		}
		if strings.TrimSpace(r.Operator) == "" {
			return fmt.Errorf("path rule operator is required")
		}
		switch strings.ToLower(strings.TrimSpace(r.Operator)) {
		case "empty", "not_empty":
			return nil
		case "=", "!=", ">", "<", ">=", "<=":
			if r.Expected == nil {
				return fmt.Errorf("path rule expected is required")
			}
			return nil
		default:
			return fmt.Errorf("path rule operator %q is not supported", r.Operator)
		}
	default:
		return fmt.Errorf("rule type %q is not supported", r.Type)
	}
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
