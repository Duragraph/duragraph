package humanloop

import "time"

// Event types
const (
	EventTypeInterruptCreated  = "interrupt.created"
	EventTypeInterruptResolved = "interrupt.resolved"
)

// InterruptCreated event
type InterruptCreated struct {
	InterruptID string                   `json:"interrupt_id"`
	RunID       string                   `json:"run_id"`
	NodeID      string                   `json:"node_id"`
	Reason      string                   `json:"reason"`
	State       map[string]interface{}   `json:"state"`
	ToolCalls   []map[string]interface{} `json:"tool_calls,omitempty"`
	OccurredAt  time.Time                `json:"occurred_at"`
}

func (e InterruptCreated) EventType() string     { return EventTypeInterruptCreated }
func (e InterruptCreated) AggregateID() string   { return e.InterruptID }
func (e InterruptCreated) AggregateType() string { return "interrupt" }

// InterruptResolved event
type InterruptResolved struct {
	InterruptID string                   `json:"interrupt_id"`
	RunID       string                   `json:"run_id"`
	ToolOutputs []map[string]interface{} `json:"tool_outputs,omitempty"`
	OccurredAt  time.Time                `json:"occurred_at"`
}

func (e InterruptResolved) EventType() string     { return EventTypeInterruptResolved }
func (e InterruptResolved) AggregateID() string   { return e.InterruptID }
func (e InterruptResolved) AggregateType() string { return "interrupt" }
