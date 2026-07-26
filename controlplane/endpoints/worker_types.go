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
// run.started | run.completed | run.failed | execution.node_started |
// execution.node_completed | execution.node_failed. LeaseEpoch fences all
// non-start events; run.started ignores it (it establishes the lease).
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
