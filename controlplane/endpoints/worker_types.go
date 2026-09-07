// Hand-defined worker↔control-plane protocol types. This is an internal,
// native protocol (NOT the public LangGraph API), so its types live here rather
// than in duragraph-latest.yaml. Source of truth: spec/models/d2 workers block
// + workers.d2 + the worker-execution design doc.
package endpoints

import (
	"encoding/json"

	"github.com/google/uuid"
)

type WorkerRegisterRequest struct {
	WorkerID uuid.UUID `json:"worker_id"`
	Graphs   []string  `json:"graphs"`
	Capacity int       `json:"capacity"`

	// GraphDefinitions carries the actual graph bodies the worker can run.
	//
	// system-architecture.d2 routes graph_entity.graph_def to this endpoint —
	// "SDK registers graph definition" — and it is the ONLY declared way a
	// definition enters the system: nothing else in the API writes to `graphs`
	// (the two graph routes are both GETs). Without it a graph could only be
	// installed with direct SQL, so a user with nothing but the HTTP API could
	// create an assistant, a thread and a run, and the run could never execute.
	//
	// Optional and additive: `graphs` (names only) still registers a worker for
	// graphs that already exist, which is what an executor-only worker does.
	GraphDefinitions []GraphDefinitionInput `json:"graph_definitions,omitempty"`
}

// GraphDefinitionInput is one graph body as the SDK registers it. The shape
// mirrors the graphs table (name, version, description, nodes, edges, config)
// and WorkerGraphResponse, which is what the worker reads back at execution
// time — the same definition making a round trip.
type GraphDefinitionInput struct {
	Name        string          `json:"name"`
	Version     string          `json:"version,omitempty"`
	Description string          `json:"description,omitempty"`
	AssistantID *uuid.UUID      `json:"assistant_id,omitempty"`
	Nodes       json.RawMessage `json:"nodes"`
	Edges       json.RawMessage `json:"edges"`
	Config      json.RawMessage `json:"config,omitempty"`
}

type WorkerRegisterResponse struct {
	WorkerID uuid.UUID `json:"worker_id"`
	Status   string    `json:"status"`
}

type WorkerHeartbeatRequest struct {
	Status     string `json:"status"`
	ActiveRuns int    `json:"active_runs"`
}

type WorkerHeartbeatResponse struct {
	Commands []string `json:"commands"`
}

// WorkerEvent is one worker→server state event. Type is one of:
// run.started | run.completed | run.failed | run.requires_action |
// execution.node_started | execution.node_completed | execution.node_failed.
// LeaseEpoch fences all non-start events; run.started ignores it (it
// establishes the lease).
//
// run.requires_action carries the HITL suspension: Reason is the interrupt
// reason CHECK value ('tool_call'|'approval_required'|'input_needed'), State is
// the checkpointed channel state at the pause point, and ToolCalls is the
// optional pending tool-call payload. NodeID names the node the run paused at.
type WorkerEvent struct {
	Type       string          `json:"type"`
	LeaseEpoch int             `json:"lease_epoch"`
	NodeID     string          `json:"node_id,omitempty"`
	NodeType   string          `json:"node_type,omitempty"`
	NodeStatus string          `json:"node_status,omitempty"`
	Input      json.RawMessage `json:"input,omitempty"`
	Output     json.RawMessage `json:"output,omitempty"`
	DurationMs *int            `json:"duration_ms,omitempty"`
	Error      *string         `json:"error,omitempty"`
	Reason     string          `json:"reason,omitempty"`
	State      json.RawMessage `json:"state,omitempty"`
	ToolCalls  json.RawMessage `json:"tool_calls,omitempty"`
}

type WorkerEventsRequest struct {
	Events []WorkerEvent `json:"events"`
}

type RunStartedResponse struct {
	LeaseEpoch int `json:"lease_epoch"`
}

type CheckpointWriteRequest struct {
	RunID      uuid.UUID       `json:"run_id"`
	LeaseEpoch int             `json:"lease_epoch"`
	Version    int             `json:"version"`
	State      json.RawMessage `json:"state"`
}

type CheckpointWriteResponse struct {
	CheckpointID int64 `json:"checkpoint_id"`
}

type CheckpointResponse struct {
	CheckpointID int64           `json:"checkpoint_id"`
	RunID        uuid.UUID       `json:"run_id"`
	Version      int             `json:"version"`
	State        json.RawMessage `json:"state"`
}

// WorkerGraphResponse is the graph definition a worker loads to execute a run:
// the raw nodes/edges/config JSON from the graphs table, forwarded verbatim.
// The worker parses these into its own worker.GraphDefinition, so the server
// never needs the interpreter's types (keeps the HTTP decoupling intact).
type WorkerGraphResponse struct {
	Nodes  json.RawMessage `json:"nodes"`
	Edges  json.RawMessage `json:"edges"`
	Config json.RawMessage `json:"config"`
}
