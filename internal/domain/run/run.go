package run

import (
	"time"

	"github.com/duragraph/duragraph/internal/pkg/errors"
	"github.com/duragraph/duragraph/internal/pkg/eventbus"
	pkguuid "github.com/duragraph/duragraph/internal/pkg/uuid"
)

// Run represents a workflow run aggregate
type Run struct {
	id          string
	threadID    string
	assistantID string
	status      Status
	input       map[string]interface{}
	output      map[string]interface{}
	error       string
	metadata    map[string]interface{}
	createdAt   time.Time
	startedAt   *time.Time
	completedAt *time.Time
	updatedAt   time.Time

	// Uncommitted events
	events []eventbus.Event
}

// NewRun creates a new Run aggregate
func NewRun(threadID, assistantID string, input map[string]interface{}) (*Run, error) {
	if threadID == "" {
		return nil, errors.InvalidInput("thread_id", "thread_id is required")
	}
	if assistantID == "" {
		return nil, errors.InvalidInput("assistant_id", "assistant_id is required")
	}

	now := time.Now()
	runID := pkguuid.New()

	run := &Run{
		id:          runID,
		threadID:    threadID,
		assistantID: assistantID,
		status:      StatusQueued,
		input:       input,
		metadata:    make(map[string]interface{}),
		createdAt:   now,
		updatedAt:   now,
		events:      make([]eventbus.Event, 0),
	}

	// Record domain event
	run.recordEvent(RunCreated{
		RunID:       runID,
		ThreadID:    threadID,
		AssistantID: assistantID,
		Input:       input,
		OccurredAt:  now,
	})

	return run, nil
}

// ID returns the run ID
func (r *Run) ID() string {
	return r.id
}

// ThreadID returns the thread ID
func (r *Run) ThreadID() string {
	return r.threadID
}

// AssistantID returns the assistant ID
func (r *Run) AssistantID() string {
	return r.assistantID
}

// Status returns the current status
func (r *Run) Status() Status {
	return r.status
}

// Input returns the input
func (r *Run) Input() map[string]interface{} {
	return r.input
}

// Output returns the output
func (r *Run) Output() map[string]interface{} {
	return r.output
}

// Error returns the error message
func (r *Run) Error() string {
	return r.error
}

// Metadata returns the metadata
func (r *Run) Metadata() map[string]interface{} {
	return r.metadata
}

// CreatedAt returns the creation time
func (r *Run) CreatedAt() time.Time {
	return r.createdAt
}

// StartedAt returns the start time
func (r *Run) StartedAt() *time.Time {
	return r.startedAt
}

// CompletedAt returns the completion time
func (r *Run) CompletedAt() *time.Time {
	return r.completedAt
}

// UpdatedAt returns the last update time
func (r *Run) UpdatedAt() time.Time {
	return r.updatedAt
}

// Start transitions the run to in-progress status
func (r *Run) Start() error {
	if !r.status.CanTransitionTo(StatusInProgress) {
		return errors.InvalidState(r.status.String(), "start")
	}

	now := time.Now()
	r.status = StatusInProgress
	r.startedAt = &now
	r.updatedAt = now

	r.recordEvent(RunStarted{
		RunID:      r.id,
		OccurredAt: now,
	})

	return nil
}

// Complete transitions the run to completed status
func (r *Run) Complete(output map[string]interface{}) error {
	if !r.status.CanTransitionTo(StatusCompleted) {
		return errors.InvalidState(r.status.String(), "complete")
	}

	now := time.Now()
	r.status = StatusCompleted
	r.output = output
	r.completedAt = &now
	r.updatedAt = now

	r.recordEvent(RunCompleted{
		RunID:      r.id,
		Output:     output,
		OccurredAt: now,
	})

	return nil
}

// Fail transitions the run to failed status
func (r *Run) Fail(errMsg string) error {
	if !r.status.CanTransitionTo(StatusFailed) {
		return errors.InvalidState(r.status.String(), "fail")
	}

	now := time.Now()
	r.status = StatusFailed
	r.error = errMsg
	r.completedAt = &now
	r.updatedAt = now

	r.recordEvent(RunFailed{
		RunID:      r.id,
		Error:      errMsg,
		OccurredAt: now,
	})

	return nil
}

// Cancel transitions the run to cancelled status
func (r *Run) Cancel(reason string) error {
	if !r.status.CanTransitionTo(StatusCancelled) {
		return errors.InvalidState(r.status.String(), "cancel")
	}

	now := time.Now()
	r.status = StatusCancelled
	r.completedAt = &now
	r.updatedAt = now

	r.recordEvent(RunCancelled{
		RunID:      r.id,
		Reason:     reason,
		OccurredAt: now,
	})

	return nil
}

// RequiresAction transitions the run to requires_action status
func (r *Run) RequiresAction(interruptID, reason string, toolCalls []map[string]interface{}) error {
	if !r.status.CanTransitionTo(StatusRequiresAction) {
		return errors.InvalidState(r.status.String(), "requires_action")
	}

	now := time.Now()
	r.status = StatusRequiresAction
	r.updatedAt = now

	r.recordEvent(RunRequiresAction{
		RunID:       r.id,
		InterruptID: interruptID,
		Reason:      reason,
		ToolCalls:   toolCalls,
		OccurredAt:  now,
	})

	return nil
}

// Resume transitions the run back to in-progress after action
func (r *Run) Resume(interruptID string, toolOutputs []map[string]interface{}) error {
	if !r.status.CanTransitionTo(StatusInProgress) {
		return errors.InvalidState(r.status.String(), "resume")
	}

	now := time.Now()
	r.status = StatusInProgress
	r.updatedAt = now

	r.recordEvent(RunResumed{
		RunID:       r.id,
		InterruptID: interruptID,
		ToolOutputs: toolOutputs,
		OccurredAt:  now,
	})

	return nil
}

// Events returns the uncommitted events
func (r *Run) Events() []eventbus.Event {
	return r.events
}

// ClearEvents clears the uncommitted events
func (r *Run) ClearEvents() {
	r.events = make([]eventbus.Event, 0)
}

// recordEvent adds an event to the uncommitted events list
func (r *Run) recordEvent(event eventbus.Event) {
	r.events = append(r.events, event)
}

// Reconstruct rebuilds run state from events (for event sourcing)
func Reconstruct(events []eventbus.Event) (*Run, error) {
	if len(events) == 0 {
		return nil, errors.InvalidInput("events", "at least one event is required")
	}

	run := &Run{
		events: make([]eventbus.Event, 0),
	}

	for _, event := range events {
		if err := run.applyEvent(event); err != nil {
			return nil, err
		}
	}

	return run, nil
}

// applyEvent applies an event to reconstruct state
func (r *Run) applyEvent(event eventbus.Event) error {
	switch e := event.(type) {
	case RunCreated:
		r.id = e.RunID
		r.threadID = e.ThreadID
		r.assistantID = e.AssistantID
		r.input = e.Input
		r.status = StatusQueued
		r.createdAt = e.OccurredAt
		r.updatedAt = e.OccurredAt
		r.metadata = e.Metadata
		if r.metadata == nil {
			r.metadata = make(map[string]interface{})
		}

	case RunStarted:
		r.status = StatusInProgress
		r.startedAt = &e.OccurredAt
		r.updatedAt = e.OccurredAt

	case RunCompleted:
		r.status = StatusCompleted
		r.output = e.Output
		r.completedAt = &e.OccurredAt
		r.updatedAt = e.OccurredAt

	case RunFailed:
		r.status = StatusFailed
		r.error = e.Error
		r.completedAt = &e.OccurredAt
		r.updatedAt = e.OccurredAt

	case RunCancelled:
		r.status = StatusCancelled
		r.completedAt = &e.OccurredAt
		r.updatedAt = e.OccurredAt

	case RunRequiresAction:
		r.status = StatusRequiresAction
		r.updatedAt = e.OccurredAt

	case RunResumed:
		r.status = StatusInProgress
		r.updatedAt = e.OccurredAt
	}

	return nil
}
