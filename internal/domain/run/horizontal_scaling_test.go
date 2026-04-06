package run

import (
	"testing"
	"time"
)

func TestRunVersioning(t *testing.T) {
	r, err := NewRun("thread-1", "assistant-1", map[string]interface{}{"q": "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if r.Version() != 1 {
		t.Errorf("expected version 1, got %d", r.Version())
	}

	r.IncrementVersion()
	if r.Version() != 2 {
		t.Errorf("expected version 2, got %d", r.Version())
	}
}

func TestRunLeaseEpoch(t *testing.T) {
	r, err := NewRun("thread-1", "assistant-1", map[string]interface{}{"q": "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if r.LeaseEpoch() != 0 {
		t.Errorf("expected lease epoch 0, got %d", r.LeaseEpoch())
	}

	r.AssignToWorker("worker-1", 2*time.Minute)
	if r.LeaseEpoch() != 1 {
		t.Errorf("expected lease epoch 1 after assign, got %d", r.LeaseEpoch())
	}

	r.IncrementRetry()
	if r.LeaseEpoch() != 2 {
		t.Errorf("expected lease epoch 2 after retry, got %d", r.LeaseEpoch())
	}
	if r.WorkerID() != "" {
		t.Errorf("expected empty worker ID after retry, got %q", r.WorkerID())
	}

	r.AssignToWorker("worker-2", 2*time.Minute)
	if r.LeaseEpoch() != 3 {
		t.Errorf("expected lease epoch 3 after re-assign, got %d", r.LeaseEpoch())
	}
}

func TestReconstructFromDataWithVersion(t *testing.T) {
	data := RunData{
		ID:          "run-1",
		ThreadID:    "thread-1",
		AssistantID: "assistant-1",
		Status:      "in_progress",
		Version:     5,
		LeaseEpoch:  3,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	r := ReconstructFromData(data)
	if r.Version() != 5 {
		t.Errorf("expected version 5, got %d", r.Version())
	}
	if r.LeaseEpoch() != 3 {
		t.Errorf("expected lease epoch 3, got %d", r.LeaseEpoch())
	}
}

func TestReconstructFromDataVersionDefaultsTo1(t *testing.T) {
	data := RunData{
		ID:          "run-1",
		ThreadID:    "thread-1",
		AssistantID: "assistant-1",
		Status:      "queued",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	r := ReconstructFromData(data)
	if r.Version() != 1 {
		t.Errorf("expected version to default to 1, got %d", r.Version())
	}
}
