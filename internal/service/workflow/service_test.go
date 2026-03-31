package workflow

import (
	"errors"
	"testing"
	"time"

	"github.com/wiregoblin/wiregoblin/internal/model"
)

func TestEmitWorkflowFinishedRedactsSecrets(t *testing.T) {
	t.Parallel()

	events := make(chan model.RunEvent, 1)
	service := &Service{}

	service.emitWorkflowFinished(
		events,
		"Demo",
		"Alpha",
		1,
		time.Now(),
		errors.New("token secret-123 leaked"),
		[]string{"secret-123"},
	)

	event := <-events
	if event.Status != "failed" {
		t.Fatalf("status = %q, want failed", event.Status)
	}
	if event.Error != "token [REDACTED] leaked" {
		t.Fatalf("error = %q", event.Error)
	}
}

func TestSecretValuesFromProjectIncludesSecretVariables(t *testing.T) {
	values := secretValuesFromProject(&model.Project{
		Secrets:         []model.Entry{{Key: "api_token", Value: "secret-123"}},
		SecretVariables: []model.Entry{{Key: "session_token", Value: "runtime-456"}},
	})

	if len(values) != 2 {
		t.Fatalf("len(values) = %d, want 2", len(values))
	}
	if values[0] != "runtime-456" && values[1] != "runtime-456" {
		t.Fatalf("runtime secret variable missing from %v", values)
	}
}
