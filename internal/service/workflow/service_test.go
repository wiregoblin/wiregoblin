package workflowservice

import (
	"errors"
	"testing"
	"time"

	"github.com/wiregoblin/wiregoblin/internal/models"
)

func TestEmitWorkflowFinishedRedactsSecrets(t *testing.T) {
	t.Parallel()

	events := make(chan models.RunEvent, 1)
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
