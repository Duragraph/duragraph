package humanloop

import (
	"time"

	"github.com/duragraph/duragraph/internal/pkg/errors"
	"github.com/duragraph/duragraph/internal/pkg/eventbus"
	pkguuid "github.com/duragraph/duragraph/internal/pkg/uuid"
)

// InterruptReason represents the reason for an interrupt
type InterruptReason string

const (
	ReasonToolCall         InterruptReason = "tool_call"
	ReasonApprovalRequired InterruptReason = "approval_required"
	ReasonInputNeeded      InterruptReason = "input_needed"
)

// Interrupt represents a human-in-loop interrupt aggregate
type Interrupt struct {
	id         string
	runID      string
	nodeID     string
	reason     InterruptReason
	state      map[string]interface{}
	toolCalls  []map[string]interface{}
	resolved   bool
	resolvedAt *time.Time
	createdAt  time.Time

	// Uncommitted events
	events []eventbus.Event
}

// NewInterrupt creates a new Interrupt aggregate
func NewInterrupt(runID, nodeID string, reason InterruptReason, state map[string]interface{}, toolCalls []map[string]interface{}) (*Interrupt, error) {
	if runID == "" {
		return nil, errors.InvalidInput("run_id", "run_id is required")
	}
	if nodeID == "" {
		return nil, errors.InvalidInput("node_id", "node_id is required")
	}

	now := time.Now()
	interruptID := pkguuid.New()

	if state == nil {
		state = make(map[string]interface{})
	}
	if toolCalls == nil {
		toolCalls = make([]map[string]interface{}, 0)
	}

	interrupt := &Interrupt{
		id:        interruptID,
		runID:     runID,
		nodeID:    nodeID,
		reason:    reason,
		state:     state,
		toolCalls: toolCalls,
		resolved:  false,
		createdAt: now,
		events:    make([]eventbus.Event, 0),
	}

	interrupt.recordEvent(InterruptCreated{
		InterruptID: interruptID,
		RunID:       runID,
		NodeID:      nodeID,
		Reason:      string(reason),
		State:       state,
		ToolCalls:   toolCalls,
		OccurredAt:  now,
	})

	return interrupt, nil
}

// ID returns the interrupt ID
func (i *Interrupt) ID() string {
	return i.id
}

// RunID returns the run ID
func (i *Interrupt) RunID() string {
	return i.runID
}

// NodeID returns the node ID
func (i *Interrupt) NodeID() string {
	return i.nodeID
}

// Reason returns the interrupt reason
func (i *Interrupt) Reason() InterruptReason {
	return i.reason
}

// State returns the state at interrupt
func (i *Interrupt) State() map[string]interface{} {
	return i.state
}

// ToolCalls returns the tool calls
func (i *Interrupt) ToolCalls() []map[string]interface{} {
	return i.toolCalls
}

// IsResolved returns whether the interrupt has been resolved
func (i *Interrupt) IsResolved() bool {
	return i.resolved
}

// ResolvedAt returns the resolution time
func (i *Interrupt) ResolvedAt() *time.Time {
	return i.resolvedAt
}

// CreatedAt returns the creation time
func (i *Interrupt) CreatedAt() time.Time {
	return i.createdAt
}

// Resolve resolves the interrupt with tool outputs
func (i *Interrupt) Resolve(toolOutputs []map[string]interface{}) error {
	if i.resolved {
		return errors.InvalidState("resolved", "resolve")
	}

	now := time.Now()
	i.resolved = true
	i.resolvedAt = &now

	i.recordEvent(InterruptResolved{
		InterruptID: i.id,
		RunID:       i.runID,
		ToolOutputs: toolOutputs,
		OccurredAt:  now,
	})

	return nil
}

// Events returns the uncommitted events
func (i *Interrupt) Events() []eventbus.Event {
	return i.events
}

// ClearEvents clears the uncommitted events
func (i *Interrupt) ClearEvents() {
	i.events = make([]eventbus.Event, 0)
}

// recordEvent adds an event to the uncommitted events list
func (i *Interrupt) recordEvent(event eventbus.Event) {
	i.events = append(i.events, event)
}
