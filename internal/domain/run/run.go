package run

import (
	"time"

	"github.com/duragraph/duragraph/internal/pkg/errors"
	"github.com/duragraph/duragraph/internal/pkg/eventbus"
	pkguuid "github.com/duragraph/duragraph/internal/pkg/uuid"
)

// Run represents a workflow run aggregate
type Run struct {
	id                string
	threadID          string
	assistantID       string
	status            Status
	input             map[string]interface{}
	output            map[string]interface{}
	config            map[string]interface{}
	error             string
	metadata          map[string]interface{}
	multitaskStrategy string
	createdAt         time.Time
	startedAt         *time.Time
	completedAt       *time.Time
	updatedAt         time.Time

	// Uncommitted events
	events []eventbus.Event
}

// RunOptions holds optional parameters for creating a run
type RunOptions struct {
	Config            map[string]interface{}
	Metadata          map[string]interface{}
	MultitaskStrategy string
}

// NewRun creates a new Run aggregate
func NewRun(threadID, assistantID string, input map[string]interface{}, opts ...RunOptions) (*Run, error) {
	if threadID == "" {
		return nil, errors.InvalidInput("thread_id", "thread_id is required")
	}
	if assistantID == "" {
		return nil, errors.InvalidInput("assistant_id", "assistant_id is required")
	}

	now := time.Now()
	runID := pkguuid.New()

	// Apply options
	var options RunOptions
	if len(opts) > 0 {
		options = opts[0]
	}

	config := options.Config
	if config == nil {
		config = make(map[string]interface{})
	}

	metadata := options.Metadata
	if metadata == nil {
		metadata = make(map[string]interface{})
	}

	multitaskStrategy := options.MultitaskStrategy
	if multitaskStrategy == "" {
		multitaskStrategy = "reject" // Default strategy
	}

	run := &Run{
		id:                runID,
		threadID:          threadID,
		assistantID:       assistantID,
		status:            StatusQueued,
		input:             input,
		config:            config,
		metadata:          metadata,
		multitaskStrategy: multitaskStrategy,
		createdAt:         now,
		updatedAt:         now,
		events:            make([]eventbus.Event, 0),
	}

	// Record domain event
	run.recordEvent(RunCreated{
		RunID:             runID,
		ThreadID:          threadID,
		AssistantID:       assistantID,
		Input:             input,
		Config:            config,
		Metadata:          metadata,
		MultitaskStrategy: multitaskStrategy,
		OccurredAt:        now,
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

// Config returns the config
func (r *Run) Config() map[string]interface{} {
	return r.config
}

// MultitaskStrategy returns the multitask strategy
func (r *Run) MultitaskStrategy() string {
	return r.multitaskStrategy
}

// RecursionLimit returns the recursion limit from config (default 25)
func (r *Run) RecursionLimit() int {
	if r.config == nil {
		return 25
	}
	if limit, ok := r.config["recursion_limit"].(float64); ok {
		return int(limit)
	}
	if limit, ok := r.config["recursion_limit"].(int); ok {
		return limit
	}
	return 25
}

// Tags returns the tags from config
func (r *Run) Tags() []string {
	if r.config == nil {
		return nil
	}
	tagsRaw, ok := r.config["tags"]
	if !ok {
		return nil
	}
	switch t := tagsRaw.(type) {
	case []string:
		return t
	case []interface{}:
		tags := make([]string, 0, len(t))
		for _, v := range t {
			if s, ok := v.(string); ok {
				tags = append(tags, s)
			}
		}
		return tags
	}
	return nil
}

// Configurable returns the configurable fields from config
func (r *Run) Configurable() map[string]interface{} {
	if r.config == nil {
		return nil
	}
	if configurable, ok := r.config["configurable"].(map[string]interface{}); ok {
		return configurable
	}
	return nil
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
		r.config = e.Config
		r.status = StatusQueued
		r.createdAt = e.OccurredAt
		r.updatedAt = e.OccurredAt
		r.metadata = e.Metadata
		if r.metadata == nil {
			r.metadata = make(map[string]interface{})
		}
		if r.config == nil {
			r.config = make(map[string]interface{})
		}
		r.multitaskStrategy = e.MultitaskStrategy
		if r.multitaskStrategy == "" {
			r.multitaskStrategy = "reject"
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

// RunData holds raw data for reconstructing a Run from database projection
type RunData struct {
	ID                string
	ThreadID          string
	AssistantID       string
	Status            string
	Input             map[string]interface{}
	Output            map[string]interface{}
	Config            map[string]interface{}
	Error             string
	Metadata          map[string]interface{}
	MultitaskStrategy string
	CreatedAt         time.Time
	StartedAt         *time.Time
	CompletedAt       *time.Time
	UpdatedAt         time.Time
}

// ReconstructFromData rebuilds a Run from database projection data
func ReconstructFromData(data RunData) *Run {
	status := StatusQueued
	switch data.Status {
	case "queued":
		status = StatusQueued
	case "in_progress":
		status = StatusInProgress
	case "completed", "success":
		status = StatusCompleted
	case "failed", "error":
		status = StatusFailed
	case "cancelled":
		status = StatusCancelled
	case "requires_action":
		status = StatusRequiresAction
	case "timeout":
		status = StatusTimeout
	case "interrupted":
		status = StatusInterrupted
	}

	metadata := data.Metadata
	if metadata == nil {
		metadata = make(map[string]interface{})
	}

	config := data.Config
	if config == nil {
		config = make(map[string]interface{})
	}

	multitaskStrategy := data.MultitaskStrategy
	if multitaskStrategy == "" {
		multitaskStrategy = "reject"
	}

	return &Run{
		id:                data.ID,
		threadID:          data.ThreadID,
		assistantID:       data.AssistantID,
		status:            status,
		input:             data.Input,
		output:            data.Output,
		config:            config,
		error:             data.Error,
		metadata:          metadata,
		multitaskStrategy: multitaskStrategy,
		createdAt:         data.CreatedAt,
		startedAt:         data.StartedAt,
		completedAt:       data.CompletedAt,
		updatedAt:         data.UpdatedAt,
		events:            make([]eventbus.Event, 0),
	}
}
