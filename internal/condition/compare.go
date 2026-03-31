// Package condition provides shared comparison logic used by workflow blocks.
package condition

import (
	"fmt"
	"strconv"
	"strings"
)

// Evaluate applies one supported comparison operator to two string values.
func Evaluate(actual, operator, expected string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(operator)) {
	case "=":
		return actual == expected, nil
	case "!=":
		return actual != expected, nil
	case ">":
		return compareNumbers(actual, expected, func(a, b float64) bool { return a > b })
	case "<":
		return compareNumbers(actual, expected, func(a, b float64) bool { return a < b })
	case ">=":
		return compareNumbers(actual, expected, func(a, b float64) bool { return a >= b })
	case "<=":
		return compareNumbers(actual, expected, func(a, b float64) bool { return a <= b })
	case "like", "contains":
		return strings.Contains(strings.ToLower(actual), strings.ToLower(expected)), nil
	default:
		return false, fmt.Errorf("unsupported operator: %s", operator)
	}
}

func compareNumbers(actual, expected string, cmp func(a, b float64) bool) (bool, error) {
	left, err := strconv.ParseFloat(strings.TrimSpace(actual), 64)
	if err != nil {
		return false, fmt.Errorf("actual value is not numeric")
	}
	right, err := strconv.ParseFloat(strings.TrimSpace(expected), 64)
	if err != nil {
		return false, fmt.Errorf("expected value is not numeric")
	}
	return cmp(left, right), nil
}
