package delay

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wiregoblin/wiregoblin/internal/models"
)

func TestExecuteWaitsAndReturnsMilliseconds(t *testing.T) {
	step := models.Step{Config: map[string]any{"milliseconds": 1}}

	result, err := New().Execute(context.Background(), nil, step)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := result.Output.(map[string]any)
	if output["milliseconds"] != 1 {
		t.Fatalf("milliseconds = %v, want 1", output["milliseconds"])
	}
}

func TestExecuteHonorsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := New().Execute(ctx, nil, models.Step{Config: map[string]any{"milliseconds": 50}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want %v", err, context.Canceled)
	}
}

func TestExecuteTimesOutFromDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()

	_, err := New().Execute(ctx, nil, models.Step{Config: map[string]any{"milliseconds": 50}})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want %v", err, context.DeadlineExceeded)
	}
}
