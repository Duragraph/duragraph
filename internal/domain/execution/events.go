package execution

import "time"

// Event types
const (
	EventTypeNodeStarted   = "execution.node_started"
	EventTypeNodeCompleted = "execution.node_completed"
	EventTypeNodeFailed    = "execution.node_failed"
	EventTypeNodeSkipped   = "execution.node_skipped"
)

// NodeStarted event
type NodeStarted struct {
	RunID      string                 `json:"run_id"`
	NodeID     string                 `json:"node_id"`
	NodeType   string                 `json:"node_type"`
	Input      map[string]interface{} `json:"input,omitempty"`
	OccurredAt time.Time              `json:"occurred_at"`
}

func (e NodeStarted) EventType() string     { return EventTypeNodeStarted }
func (e NodeStarted) AggregateID() string   { return e.RunID }
func (e NodeStarted) AggregateType() string { return "execution" }

// NodeCompleted event
type NodeCompleted struct {
	RunID      string                 `json:"run_id"`
	NodeID     string                 `json:"node_id"`
	NodeType   string                 `json:"node_type"`
	Output     map[string]interface{} `json:"output,omitempty"`
	DurationMs int64                  `json:"duration_ms"`
	OccurredAt time.Time              `json:"occurred_at"`
}

func (e NodeCompleted) EventType() string     { return EventTypeNodeCompleted }
func (e NodeCompleted) AggregateID() string   { return e.RunID }
func (e NodeCompleted) AggregateType() string { return "execution" }

// NodeFailed event
type NodeFailed struct {
	RunID      string                 `json:"run_id"`
	NodeID     string                 `json:"node_id"`
	NodeType   string                 `json:"node_type"`
	Error      string                 `json:"error"`
	Input      map[string]interface{} `json:"input,omitempty"`
	OccurredAt time.Time              `json:"occurred_at"`
}

func (e NodeFailed) EventType() string     { return EventTypeNodeFailed }
func (e NodeFailed) AggregateID() string   { return e.RunID }
func (e NodeFailed) AggregateType() string { return "execution" }

// NodeSkipped event
type NodeSkipped struct {
	RunID      string    `json:"run_id"`
	NodeID     string    `json:"node_id"`
	NodeType   string    `json:"node_type"`
	Reason     string    `json:"reason"`
	OccurredAt time.Time `json:"occurred_at"`
}

func (e NodeSkipped) EventType() string     { return EventTypeNodeSkipped }
func (e NodeSkipped) AggregateID() string   { return e.RunID }
func (e NodeSkipped) AggregateType() string { return "execution" }
