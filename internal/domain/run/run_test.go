package run

import (
	"testing"
	"time"

	"github.com/duragraph/duragraph/internal/pkg/eventbus"
)

func TestNewRun(t *testing.T) {
	tests := []struct {
		name        string
		threadID    string
		assistantID string
		input       map[string]interface{}
		opts        []RunOptions
		wantErr     bool
		errContains string
	}{
		{
			name:        "valid run with minimal params",
			threadID:    "thread-123",
			assistantID: "assistant-456",
			input:       map[string]interface{}{"message": "hello"},
		},
		{
			name:        "valid run with nil input",
			threadID:    "thread-123",
			assistantID: "assistant-456",
			input:       nil,
		},
		{
			name:        "valid run with options",
			threadID:    "thread-123",
			assistantID: "assistant-456",
			input:       map[string]interface{}{"message": "hello"},
			opts: []RunOptions{{
				Config:            map[string]interface{}{"recursion_limit": 10},
				Metadata:          map[string]interface{}{"user": "test"},
				MultitaskStrategy: "enqueue",
			}},
		},
		{
			name:        "empty thread_id",
			threadID:    "",
			assistantID: "assistant-456",
			input:       map[string]interface{}{},
			wantErr:     true,
			errContains: "thread_id",
		},
		{
			name:        "empty assistant_id",
			threadID:    "thread-123",
			assistantID: "",
			input:       map[string]interface{}{},
			wantErr:     true,
			errContains: "assistant_id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run, err := NewRun(tt.threadID, tt.assistantID, tt.input, tt.opts...)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !containsStr(err.Error(), tt.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if run.ID() == "" {
				t.Error("run ID should be generated")
			}
			if run.ThreadID() != tt.threadID {
				t.Errorf("threadID = %q, want %q", run.ThreadID(), tt.threadID)
			}
			if run.AssistantID() != tt.assistantID {
				t.Errorf("assistantID = %q, want %q", run.AssistantID(), tt.assistantID)
			}
			if run.Status() != StatusQueued {
				t.Errorf("status = %q, want %q", run.Status(), StatusQueued)
			}
			if run.CreatedAt().IsZero() {
				t.Error("createdAt should be set")
			}

			events := run.Events()
			if len(events) != 1 {
				t.Fatalf("expected 1 event, got %d", len(events))
			}
			if events[0].EventType() != "run.created" {
				t.Errorf("event type = %q, want run.created", events[0].EventType())
			}
		})
	}
}

func TestNewRun_Defaults(t *testing.T) {
	run, err := NewRun("t", "a", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if run.MultitaskStrategy() != "reject" {
		t.Errorf("default multitaskStrategy = %q, want 'reject'", run.MultitaskStrategy())
	}
	if run.Config() == nil {
		t.Error("config should default to empty map, not nil")
	}
	if run.Metadata() == nil {
		t.Error("metadata should default to empty map, not nil")
	}
	if run.RecursionLimit() != 25 {
		t.Errorf("default recursionLimit = %d, want 25", run.RecursionLimit())
	}
}

func TestNewRun_Options(t *testing.T) {
	opts := RunOptions{
		Config:            map[string]interface{}{"recursion_limit": float64(50), "tags": []string{"a", "b"}},
		Metadata:          map[string]interface{}{"env": "test"},
		MultitaskStrategy: "enqueue",
	}

	run, err := NewRun("t", "a", nil, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if run.MultitaskStrategy() != "enqueue" {
		t.Errorf("multitaskStrategy = %q, want 'enqueue'", run.MultitaskStrategy())
	}
	if run.RecursionLimit() != 50 {
		t.Errorf("recursionLimit = %d, want 50", run.RecursionLimit())
	}
	tags := run.Tags()
	if len(tags) != 2 || tags[0] != "a" || tags[1] != "b" {
		t.Errorf("tags = %v, want [a b]", tags)
	}
}

func TestRun_StateTransitions(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T) *Run
		action    func(r *Run) error
		wantState Status
		wantErr   bool
	}{
		{
			name:  "queued to in_progress",
			setup: newTestRun,
			action: func(r *Run) error {
				return r.Start()
			},
			wantState: StatusInProgress,
		},
		{
			name: "in_progress to completed",
			setup: func(t *testing.T) *Run {
				r := newTestRun(t)
				must(t, r.Start())
				return r
			},
			action: func(r *Run) error {
				return r.Complete(map[string]interface{}{"result": "ok"})
			},
			wantState: StatusCompleted,
		},
		{
			name: "in_progress to failed",
			setup: func(t *testing.T) *Run {
				r := newTestRun(t)
				must(t, r.Start())
				return r
			},
			action: func(r *Run) error {
				return r.Fail("something broke")
			},
			wantState: StatusFailed,
		},
		{
			name:  "queued to cancelled",
			setup: newTestRun,
			action: func(r *Run) error {
				return r.Cancel("user requested")
			},
			wantState: StatusCancelled,
		},
		{
			name: "in_progress to requires_action",
			setup: func(t *testing.T) *Run {
				r := newTestRun(t)
				must(t, r.Start())
				return r
			},
			action: func(r *Run) error {
				return r.RequiresAction("int-1", "need approval", nil)
			},
			wantState: StatusRequiresAction,
		},
		{
			name: "requires_action to in_progress (resume)",
			setup: func(t *testing.T) *Run {
				r := newTestRun(t)
				must(t, r.Start())
				must(t, r.RequiresAction("int-1", "need approval", nil))
				return r
			},
			action: func(r *Run) error {
				return r.Resume("int-1", nil)
			},
			wantState: StatusInProgress,
		},
		{
			name: "completed cannot start",
			setup: func(t *testing.T) *Run {
				r := newTestRun(t)
				must(t, r.Start())
				must(t, r.Complete(nil))
				return r
			},
			action: func(r *Run) error {
				return r.Start()
			},
			wantErr: true,
		},
		{
			name: "failed cannot complete",
			setup: func(t *testing.T) *Run {
				r := newTestRun(t)
				must(t, r.Start())
				must(t, r.Fail("err"))
				return r
			},
			action: func(r *Run) error {
				return r.Complete(nil)
			},
			wantErr: true,
		},
		{
			name:  "queued cannot complete directly",
			setup: newTestRun,
			action: func(r *Run) error {
				return r.Complete(nil)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := tt.setup(t)
			err := tt.action(run)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if run.Status() != tt.wantState {
				t.Errorf("status = %q, want %q", run.Status(), tt.wantState)
			}
		})
	}
}

func TestRun_CompleteSetsTimes(t *testing.T) {
	run := newTestRun(t)
	must(t, run.Start())

	if run.StartedAt() == nil {
		t.Fatal("startedAt should be set after Start()")
	}

	must(t, run.Complete(map[string]interface{}{"done": true}))

	if run.CompletedAt() == nil {
		t.Fatal("completedAt should be set after Complete()")
	}
	if run.Output()["done"] != true {
		t.Error("output should be set after Complete()")
	}
}

func TestRun_FailSetsError(t *testing.T) {
	run := newTestRun(t)
	must(t, run.Start())
	must(t, run.Fail("timeout exceeded"))

	if run.Error() != "timeout exceeded" {
		t.Errorf("error = %q, want 'timeout exceeded'", run.Error())
	}
	if run.CompletedAt() == nil {
		t.Fatal("completedAt should be set after Fail()")
	}
}

func TestRun_EventEmission(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(t *testing.T) *Run
		wantEvents []string
	}{
		{
			name:       "creation emits run.created",
			setup:      newTestRun,
			wantEvents: []string{"run.created"},
		},
		{
			name: "start emits run.created + run.started",
			setup: func(t *testing.T) *Run {
				r := newTestRun(t)
				must(t, r.Start())
				return r
			},
			wantEvents: []string{"run.created", "run.started"},
		},
		{
			name: "full lifecycle",
			setup: func(t *testing.T) *Run {
				r := newTestRun(t)
				must(t, r.Start())
				must(t, r.Complete(nil))
				return r
			},
			wantEvents: []string{"run.created", "run.started", "run.completed"},
		},
		{
			name: "failure lifecycle",
			setup: func(t *testing.T) *Run {
				r := newTestRun(t)
				must(t, r.Start())
				must(t, r.Fail("err"))
				return r
			},
			wantEvents: []string{"run.created", "run.started", "run.failed"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := tt.setup(t)
			events := run.Events()
			if len(events) != len(tt.wantEvents) {
				t.Fatalf("got %d events, want %d", len(events), len(tt.wantEvents))
			}
			for i, want := range tt.wantEvents {
				if events[i].EventType() != want {
					t.Errorf("event[%d] = %q, want %q", i, events[i].EventType(), want)
				}
			}
		})
	}
}

func TestRun_ClearEvents(t *testing.T) {
	run := newTestRun(t)
	if len(run.Events()) == 0 {
		t.Fatal("should have events after creation")
	}
	run.ClearEvents()
	if len(run.Events()) != 0 {
		t.Errorf("events should be empty after ClearEvents(), got %d", len(run.Events()))
	}
}

func TestRun_AssignToWorker(t *testing.T) {
	run := newTestRun(t)
	lease := 2 * time.Minute

	if run.WorkerID() != "" {
		t.Error("workerID should be empty initially")
	}
	if run.LeaseExpiresAt() != nil {
		t.Error("leaseExpiresAt should be nil initially")
	}

	before := time.Now()
	run.AssignToWorker("worker-1", lease)
	after := time.Now()

	if run.WorkerID() != "worker-1" {
		t.Errorf("workerID = %q, want 'worker-1'", run.WorkerID())
	}
	if run.LeaseExpiresAt() == nil {
		t.Fatal("leaseExpiresAt should be set")
	}
	if run.LeaseExpiresAt().Before(before.Add(lease)) || run.LeaseExpiresAt().After(after.Add(lease)) {
		t.Error("leaseExpiresAt should be ~now + lease duration")
	}
	if run.LastHeartbeatAt() == nil {
		t.Fatal("lastHeartbeatAt should be set")
	}
}

func TestRun_WorkerHeartbeat(t *testing.T) {
	run := newTestRun(t)
	lease := 2 * time.Minute
	run.AssignToWorker("worker-1", lease)

	firstExpiry := *run.LeaseExpiresAt()
	time.Sleep(time.Millisecond)

	run.WorkerHeartbeat(lease)

	if run.LeaseExpiresAt().Before(firstExpiry) {
		t.Error("heartbeat should extend the lease")
	}
	if run.LastHeartbeatAt() == nil {
		t.Fatal("lastHeartbeatAt should be updated")
	}
}

func TestRun_IncrementRetry(t *testing.T) {
	run := newTestRun(t)
	run.AssignToWorker("worker-1", 2*time.Minute)

	if run.RetryCount() != 0 {
		t.Errorf("retryCount = %d, want 0", run.RetryCount())
	}

	run.IncrementRetry()

	if run.RetryCount() != 1 {
		t.Errorf("retryCount = %d, want 1", run.RetryCount())
	}
	if run.WorkerID() != "" {
		t.Errorf("workerID should be cleared after retry, got %q", run.WorkerID())
	}
	if run.LeaseExpiresAt() != nil {
		t.Error("leaseExpiresAt should be nil after retry")
	}
	if run.LastHeartbeatAt() != nil {
		t.Error("lastHeartbeatAt should be nil after retry")
	}

	run.IncrementRetry()
	if run.RetryCount() != 2 {
		t.Errorf("retryCount = %d, want 2 after second increment", run.RetryCount())
	}
}

func TestRun_ReconstructFromData(t *testing.T) {
	now := time.Now()
	started := now.Add(-time.Minute)
	lease := now.Add(2 * time.Minute)
	heartbeat := now.Add(-10 * time.Second)

	tests := []struct {
		name string
		data RunData
		want func(t *testing.T, r *Run)
	}{
		{
			name: "basic fields",
			data: RunData{
				ID:          "run-1",
				ThreadID:    "t-1",
				AssistantID: "a-1",
				Status:      "queued",
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			want: func(t *testing.T, r *Run) {
				if r.ID() != "run-1" {
					t.Errorf("ID = %q, want 'run-1'", r.ID())
				}
				if r.Status() != StatusQueued {
					t.Errorf("status = %q, want queued", r.Status())
				}
				if r.MultitaskStrategy() != "reject" {
					t.Errorf("default multitaskStrategy = %q, want 'reject'", r.MultitaskStrategy())
				}
			},
		},
		{
			name: "worker tracking fields",
			data: RunData{
				ID:              "run-2",
				ThreadID:        "t-1",
				AssistantID:     "a-1",
				Status:          "in_progress",
				WorkerID:        "worker-5",
				RetryCount:      3,
				LeaseExpiresAt:  &lease,
				LastHeartbeatAt: &heartbeat,
				CreatedAt:       now,
				StartedAt:       &started,
				UpdatedAt:       now,
			},
			want: func(t *testing.T, r *Run) {
				if r.WorkerID() != "worker-5" {
					t.Errorf("workerID = %q, want 'worker-5'", r.WorkerID())
				}
				if r.RetryCount() != 3 {
					t.Errorf("retryCount = %d, want 3", r.RetryCount())
				}
				if r.LeaseExpiresAt() == nil || !r.LeaseExpiresAt().Equal(lease) {
					t.Error("leaseExpiresAt not reconstructed correctly")
				}
				if r.LastHeartbeatAt() == nil || !r.LastHeartbeatAt().Equal(heartbeat) {
					t.Error("lastHeartbeatAt not reconstructed correctly")
				}
				if r.Status() != StatusInProgress {
					t.Errorf("status = %q, want in_progress", r.Status())
				}
			},
		},
		{
			name: "status aliases",
			data: RunData{
				ID:          "run-3",
				ThreadID:    "t-1",
				AssistantID: "a-1",
				Status:      "success",
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			want: func(t *testing.T, r *Run) {
				if r.Status() != StatusCompleted {
					t.Errorf("status = %q, want completed for input 'success'", r.Status())
				}
			},
		},
		{
			name: "error status alias",
			data: RunData{
				ID:          "run-4",
				ThreadID:    "t-1",
				AssistantID: "a-1",
				Status:      "error",
				Error:       "something broke",
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			want: func(t *testing.T, r *Run) {
				if r.Status() != StatusFailed {
					t.Errorf("status = %q, want failed for input 'error'", r.Status())
				}
				if r.Error() != "something broke" {
					t.Errorf("error = %q, want 'something broke'", r.Error())
				}
			},
		},
		{
			name: "nil metadata and config default to empty maps",
			data: RunData{
				ID:          "run-5",
				ThreadID:    "t-1",
				AssistantID: "a-1",
				Status:      "queued",
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			want: func(t *testing.T, r *Run) {
				if r.Config() == nil {
					t.Error("config should default to empty map")
				}
				if r.Metadata() == nil {
					t.Error("metadata should default to empty map")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := ReconstructFromData(tt.data)
			if run == nil {
				t.Fatal("ReconstructFromData returned nil")
			}
			tt.want(t, run)

			if len(run.Events()) != 0 {
				t.Error("reconstructed run should have no uncommitted events")
			}
		})
	}
}

func TestRun_Reconstruct(t *testing.T) {
	t.Run("from RunCreated event", func(t *testing.T) {
		now := time.Now()
		evts := []eventbus.Event{
			RunCreated{
				RunID:       "run-1",
				ThreadID:    "t-1",
				AssistantID: "a-1",
				Input:       map[string]interface{}{"msg": "hi"},
				OccurredAt:  now,
			},
		}

		run, err := Reconstruct(evts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if run.ID() != "run-1" {
			t.Errorf("ID = %q, want 'run-1'", run.ID())
		}
		if run.Status() != StatusQueued {
			t.Errorf("status = %q, want queued", run.Status())
		}
	})

	t.Run("empty events returns error", func(t *testing.T) {
		_, err := Reconstruct(nil)
		if err == nil {
			t.Fatal("expected error for empty events")
		}
	})
}

func TestRun_RecursionLimit(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]interface{}
		want   int
	}{
		{"nil config", nil, 25},
		{"no recursion_limit key", map[string]interface{}{}, 25},
		{"float64 value", map[string]interface{}{"recursion_limit": float64(10)}, 10},
		{"int value", map[string]interface{}{"recursion_limit": 42}, 42},
		{"string value falls back", map[string]interface{}{"recursion_limit": "10"}, 25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run, _ := NewRun("t", "a", nil, RunOptions{Config: tt.config})
			if got := run.RecursionLimit(); got != tt.want {
				t.Errorf("RecursionLimit() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestRun_Tags(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]interface{}
		want   []string
	}{
		{"nil config", nil, nil},
		{"no tags key", map[string]interface{}{}, nil},
		{"string slice", map[string]interface{}{"tags": []string{"a", "b"}}, []string{"a", "b"}},
		{"interface slice", map[string]interface{}{"tags": []interface{}{"x", "y"}}, []string{"x", "y"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run, _ := NewRun("t", "a", nil, RunOptions{Config: tt.config})
			got := run.Tags()
			if len(got) != len(tt.want) {
				t.Errorf("Tags() = %v, want %v", got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("Tags()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestRun_Configurable(t *testing.T) {
	t.Run("returns configurable map", func(t *testing.T) {
		cfg := map[string]interface{}{
			"configurable": map[string]interface{}{"thread_id": "t-1"},
		}
		run, _ := NewRun("t", "a", nil, RunOptions{Config: cfg})
		c := run.Configurable()
		if c == nil || c["thread_id"] != "t-1" {
			t.Error("Configurable() should return the configurable map")
		}
	})

	t.Run("returns nil when not present", func(t *testing.T) {
		run, _ := NewRun("t", "a", nil)
		if run.Configurable() != nil {
			t.Error("Configurable() should return nil when key absent")
		}
	})
}

// Helpers

func newTestRun(t *testing.T) *Run {
	t.Helper()
	run, err := NewRun("thread-1", "assistant-1", map[string]interface{}{"msg": "test"})
	if err != nil {
		t.Fatalf("failed to create test run: %v", err)
	}
	return run
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
