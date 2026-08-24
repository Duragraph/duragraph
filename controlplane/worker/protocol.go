// Small local copies of the worker↔control-plane wire shapes. The worker
// package must not import controlplane/endpoints (that would pull in the
// server's dependency graph), so these structs mirror the JSON tags of
// endpoints.WorkerEvent / WorkerEventsRequest / RunStartedResponse /
// CheckpointWriteRequest / CheckpointResponse (see
// controlplane/endpoints/worker_types.go) exactly, field for field.
package worker

import (
	"encoding/json"

	"github.com/google/uuid"
)

// workerEvent mirrors endpoints.WorkerEvent.
type workerEvent struct {
	Type       string          `json:"type"`
	LeaseEpoch int             `json:"lease_epoch,omitempty"`
	NodeID     string          `json:"node_id,omitempty"`
	NodeType   string          `json:"node_type,omitempty"`
	NodeStatus string          `json:"node_status,omitempty"`
	Input      json.RawMessage `json:"input,omitempty"`
	Output     json.RawMessage `json:"output,omitempty"`
	DurationMs *int            `json:"duration_ms,omitempty"`
	Error      *string         `json:"error,omitempty"`
}

// eventsRequest mirrors endpoints.WorkerEventsRequest.
type eventsRequest struct {
	Events []workerEvent `json:"events"`
}

// runStartedResponse mirrors endpoints.RunStartedResponse.
type runStartedResponse struct {
	LeaseEpoch int `json:"lease_epoch"`
}

// checkpointWriteRequest mirrors endpoints.CheckpointWriteRequest.
type checkpointWriteRequest struct {
	RunID      uuid.UUID       `json:"run_id"`
	LeaseEpoch int             `json:"lease_epoch"`
	Version    int             `json:"version"`
	State      json.RawMessage `json:"state"`
}

// checkpointResponse mirrors endpoints.CheckpointResponse (used to decode the
// GET .../checkpoints/latest response).
type checkpointResponse struct {
	CheckpointID int64           `json:"checkpoint_id"`
	RunID        uuid.UUID       `json:"run_id"`
	Version      int             `json:"version"`
	State        json.RawMessage `json:"state"`
}

// workerGraphResponse mirrors endpoints.WorkerGraphResponse (used to decode the
// GET .../runs/{rid}/graph response). Fields stay raw so LoadGraph can unmarshal
// them into the worker's own GraphDefinition.
type workerGraphResponse struct {
	Nodes  json.RawMessage `json:"nodes"`
	Edges  json.RawMessage `json:"edges"`
	Config json.RawMessage `json:"config"`
}
