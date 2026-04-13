package execution

import (
	"testing"
)

func TestEventTypes(t *testing.T) {
	tests := []struct {
		event    interface{ EventType() string }
		wantType string
	}{
		{NodeStarted{}, EventTypeNodeStarted},
		{NodeCompleted{}, EventTypeNodeCompleted},
		{NodeFailed{}, EventTypeNodeFailed},
		{NodeSkipped{}, EventTypeNodeSkipped},
	}

	for _, tc := range tests {
		t.Run(tc.wantType, func(t *testing.T) {
			if tc.event.EventType() != tc.wantType {
				t.Errorf("expected %s, got %s", tc.wantType, tc.event.EventType())
			}
		})
	}
}

func TestEventAggregateType(t *testing.T) {
	events := []interface {
		AggregateType() string
	}{
		NodeStarted{},
		NodeCompleted{},
		NodeFailed{},
		NodeSkipped{},
	}

	for _, e := range events {
		if e.AggregateType() != "execution" {
			t.Errorf("expected aggregate type 'execution', got '%s'", e.AggregateType())
		}
	}
}

func TestEventAggregateID(t *testing.T) {
	runID := "run-abc"

	events := []interface {
		AggregateID() string
	}{
		NodeStarted{RunID: runID},
		NodeCompleted{RunID: runID},
		NodeFailed{RunID: runID},
		NodeSkipped{RunID: runID},
	}

	for _, e := range events {
		if e.AggregateID() != runID {
			t.Errorf("expected aggregate ID %s, got %s", runID, e.AggregateID())
		}
	}
}
