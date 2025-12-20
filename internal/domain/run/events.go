package run

import (
	"time"
)

// Event types
const (
	EventTypeRunCreated        = "run.created"
	EventTypeRunStarted        = "run.started"
	EventTypeRunCompleted      = "run.completed"
	EventTypeRunFailed         = "run.failed"
	EventTypeRunCancelled      = "run.cancelled"
	EventTypeRunRequiresAction = "run.requires_action"
	EventTypeRunResumed        = "run.resumed"
)

// RunCreated event
type RunCreated struct {
	RunID             string                 `json:"run_id"`
	ThreadID          string                 `json:"thread_id"`
	AssistantID       string                 `json:"assistant_id"`
	Input             map[string]interface{} `json:"input,omitempty"`
	Config            map[string]interface{} `json:"config,omitempty"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
	MultitaskStrategy string                 `json:"multitask_strategy,omitempty"`
	OccurredAt        time.Time              `json:"occurred_at"`
}

func (e RunCreated) EventType() string     { return EventTypeRunCreated }
func (e RunCreated) AggregateID() string   { return e.RunID }
func (e RunCreated) AggregateType() string { return "run" }

// RunStarted event
type RunStarted struct {
	RunID      string    `json:"run_id"`
	OccurredAt time.Time `json:"occurred_at"`
}

func (e RunStarted) EventType() string     { return EventTypeRunStarted }
func (e RunStarted) AggregateID() string   { return e.RunID }
func (e RunStarted) AggregateType() string { return "run" }

// RunCompleted event
type RunCompleted struct {
	RunID      string                 `json:"run_id"`
	Output     map[string]interface{} `json:"output,omitempty"`
	OccurredAt time.Time              `json:"occurred_at"`
}

func (e RunCompleted) EventType() string     { return EventTypeRunCompleted }
func (e RunCompleted) AggregateID() string   { return e.RunID }
func (e RunCompleted) AggregateType() string { return "run" }

// RunFailed event
type RunFailed struct {
	RunID      string    `json:"run_id"`
	Error      string    `json:"error"`
	OccurredAt time.Time `json:"occurred_at"`
}

func (e RunFailed) EventType() string     { return EventTypeRunFailed }
func (e RunFailed) AggregateID() string   { return e.RunID }
func (e RunFailed) AggregateType() string { return "run" }

// RunCancelled event
type RunCancelled struct {
	RunID      string    `json:"run_id"`
	Reason     string    `json:"reason,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}

func (e RunCancelled) EventType() string     { return EventTypeRunCancelled }
func (e RunCancelled) AggregateID() string   { return e.RunID }
func (e RunCancelled) AggregateType() string { return "run" }

// RunRequiresAction event
type RunRequiresAction struct {
	RunID       string                   `json:"run_id"`
	InterruptID string                   `json:"interrupt_id"`
	Reason      string                   `json:"reason"`
	ToolCalls   []map[string]interface{} `json:"tool_calls,omitempty"`
	OccurredAt  time.Time                `json:"occurred_at"`
}

func (e RunRequiresAction) EventType() string     { return EventTypeRunRequiresAction }
func (e RunRequiresAction) AggregateID() string   { return e.RunID }
func (e RunRequiresAction) AggregateType() string { return "run" }

// RunResumed event
type RunResumed struct {
	RunID       string                   `json:"run_id"`
	InterruptID string                   `json:"interrupt_id"`
	ToolOutputs []map[string]interface{} `json:"tool_outputs,omitempty"`
	OccurredAt  time.Time                `json:"occurred_at"`
}

func (e RunResumed) EventType() string     { return EventTypeRunResumed }
func (e RunResumed) AggregateID() string   { return e.RunID }
func (e RunResumed) AggregateType() string { return "run" }
