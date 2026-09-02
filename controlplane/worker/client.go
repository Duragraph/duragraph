// HTTP client for the worker↔control-plane protocol (see
// controlplane/endpoints/workers.go for the server side). Deliberately
// decoupled from controlplane/endpoints — it only speaks HTTP/JSON, so a
// worker process never needs the server's dependency graph (Postgres pool,
// echo, etc).
package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/google/uuid"
)

// ErrStaleLease is returned by any epoch-fenced call (NodeCompleted,
// RunCompleted, RunFailed, WriteCheckpoint) when the server responds 409 —
// another worker has since re-leased the run.
var ErrStaleLease = errors.New("worker: stale lease (409)")

// Client talks to the control-plane's worker endpoints on behalf of one
// worker process.
type Client struct {
	baseURL  string
	workerID uuid.UUID
	http     *http.Client
}

// NewClient builds a Client. baseURL is the control-plane's root (e.g.
// "http://localhost:8080"); the client appends "/api/v1/..." itself.
func NewClient(baseURL string, workerID uuid.UUID, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{baseURL: baseURL, workerID: workerID, http: httpClient}
}

// Register upserts this worker as online, advertising the graphs it can run
// and its concurrency capacity. POST /api/v1/workers/register.
func (c *Client) Register(ctx context.Context, graphs []string, capacity int) error {
	req := struct {
		WorkerID uuid.UUID `json:"worker_id"`
		Graphs   []string  `json:"graphs"`
		Capacity int       `json:"capacity"`
	}{WorkerID: c.workerID, Graphs: graphs, Capacity: capacity}
	_, err := c.doJSON(ctx, http.MethodPost, "/api/v1/workers/register", req, nil)
	return err
}

// Heartbeat renews this worker's lease. POST /api/v1/workers/{id}/heartbeat.
func (c *Client) Heartbeat(ctx context.Context, activeRuns int) error {
	req := struct {
		Status     string `json:"status"`
		ActiveRuns int    `json:"active_runs"`
	}{Status: "online", ActiveRuns: activeRuns}
	_, err := c.doJSON(ctx, http.MethodPost, "/api/v1/workers/"+c.workerID.String()+"/heartbeat", req, nil)
	return err
}

// Deregister marks this worker offline, requeuing any runs it held.
// POST /api/v1/workers/{id}/deregister.
func (c *Client) Deregister(ctx context.Context) error {
	_, err := c.doJSON(ctx, http.MethodPost, "/api/v1/workers/"+c.workerID.String()+"/deregister", struct{}{}, nil)
	return err
}

// RunStarted leases runID to this worker and establishes (or bumps) its
// lease_epoch. POST /api/v1/workers/{id}/runs/{rid}/events with a single
// run.started event.
func (c *Client) RunStarted(ctx context.Context, runID uuid.UUID) (int, error) {
	body := eventsRequest{Events: []workerEvent{{Type: "run.started"}}}
	var resp runStartedResponse
	if _, err := c.doJSON(ctx, http.MethodPost, c.eventsPath(runID), body, &resp); err != nil {
		return 0, err
	}
	return resp.LeaseEpoch, nil
}

// NodeStarted opens a node's execution_history row before the node runs, fenced
// on epoch. The completing call (NodeCompleted / NodeFailed) transitions that
// same row rather than adding another — see the server's nodeEvent.
//
// Announcing the start is what makes an in-flight node visible: without it a
// node that is slow, wedged, or killed mid-execution leaves no trace at all, so
// "where is this run" is unanswerable from execution_history.
func (c *Client) NodeStarted(ctx context.Context, runID uuid.UUID, epoch int, nodeID, nodeType string) error {
	body := eventsRequest{Events: []workerEvent{{
		Type:       "execution.node_started",
		LeaseEpoch: epoch,
		NodeID:     nodeID,
		NodeType:   nodeType,
		NodeStatus: "started",
	}}}
	_, err := c.doJSON(ctx, http.MethodPost, c.eventsPath(runID), body, nil)
	return err
}

// NodeCompleted closes a node's execution_history row as succeeded, fenced on
// epoch. durationMs is the worker's own measurement of the node's execution;
// the server falls back to now()-started_at when it is nil.
func (c *Client) NodeCompleted(ctx context.Context, runID uuid.UUID, epoch int, nodeID, nodeType string, durationMs *int) error {
	body := eventsRequest{Events: []workerEvent{{
		Type:       "execution.node_completed",
		LeaseEpoch: epoch,
		NodeID:     nodeID,
		NodeType:   nodeType,
		NodeStatus: "completed",
		DurationMs: durationMs,
	}}}
	_, err := c.doJSON(ctx, http.MethodPost, c.eventsPath(runID), body, nil)
	return err
}

// NodeFailed closes a node's execution_history row as failed, recording which
// node died and why, fenced on epoch.
//
// Without it a poison node produced run.failed and nothing else: the error text
// landed on runs.error and execution_history held no row naming the node, so the
// run's own audit trail could not say where it died.
func (c *Client) NodeFailed(ctx context.Context, runID uuid.UUID, epoch int, nodeID, nodeType, reason string, durationMs *int) error {
	body := eventsRequest{Events: []workerEvent{{
		Type:       "execution.node_failed",
		LeaseEpoch: epoch,
		NodeID:     nodeID,
		NodeType:   nodeType,
		NodeStatus: "failed",
		Error:      &reason,
		DurationMs: durationMs,
	}}}
	_, err := c.doJSON(ctx, http.MethodPost, c.eventsPath(runID), body, nil)
	return err
}

// RunCompleted marks runID completed, fenced on epoch.
func (c *Client) RunCompleted(ctx context.Context, runID uuid.UUID, epoch int) error {
	body := eventsRequest{Events: []workerEvent{{Type: "run.completed", LeaseEpoch: epoch}}}
	_, err := c.doJSON(ctx, http.MethodPost, c.eventsPath(runID), body, nil)
	return err
}

// RunFailed marks runID failed with reason, fenced on epoch.
func (c *Client) RunFailed(ctx context.Context, runID uuid.UUID, epoch int, reason string) error {
	body := eventsRequest{Events: []workerEvent{{Type: "run.failed", LeaseEpoch: epoch, Error: &reason}}}
	_, err := c.doJSON(ctx, http.MethodPost, c.eventsPath(runID), body, nil)
	return err
}

// RequiresAction suspends runID for human-in-the-loop, fenced on epoch: the
// server flips the run to requires_action and records the interrupt row. nodeID
// is the node the run paused at, reason is one of the interrupts.reason CHECK
// values (tool_call|approval_required|input_needed), state is the checkpointed
// channel state at the pause point, and toolCalls is the optional pending
// tool-call payload persisted to interrupts.tool_calls (hitl.d2 on_interrupt
// step 2 "INSERT interrupt: run_id, node_id, reason, tool_calls"). A nil
// toolCalls is omitted from the request, leaving the column NULL.
func (c *Client) RequiresAction(ctx context.Context, runID uuid.UUID, epoch int, nodeID, reason string, state, toolCalls []byte) error {
	ev := workerEvent{
		Type:       "run.requires_action",
		LeaseEpoch: epoch,
		NodeID:     nodeID,
		Reason:     reason,
		State:      json.RawMessage(state),
	}
	if len(toolCalls) > 0 {
		ev.ToolCalls = json.RawMessage(toolCalls)
	}
	_, err := c.doJSON(ctx, http.MethodPost, c.eventsPath(runID), eventsRequest{Events: []workerEvent{ev}}, nil)
	return err
}

// WriteCheckpoint upserts a run snapshot, fenced on epoch, and returns the
// snapshot's id. The caller threads that id into the NEXT checkpoint's
// parent_checkpoint_id, which is what gives the checkpoint chain its lineage
// (graph-engine.d2 §4 checkpoint_mgr.metadata).
// POST /api/v1/threads/{tid}/checkpoints -> {checkpoint_id}.
func (c *Client) WriteCheckpoint(ctx context.Context, threadID, runID uuid.UUID, epoch, version int, state []byte) (int64, error) {
	body := checkpointWriteRequest{RunID: runID, LeaseEpoch: epoch, Version: version, State: json.RawMessage(state)}
	var resp checkpointWriteResponse
	if _, err := c.doJSON(ctx, http.MethodPost, "/api/v1/threads/"+threadID.String()+"/checkpoints", body, &resp); err != nil {
		return 0, err
	}
	return resp.CheckpointID, nil
}

// Checkpoint is a restored run snapshot: the snapshots row id (which becomes
// the parent_checkpoint_id of the next checkpoint written), its version, and
// the raw state jsonb carrying the checkpointState envelope.
type Checkpoint struct {
	ID      int64
	Version int
	State   []byte
}

// LatestCheckpoint fetches the highest-version snapshot for runID, for worker
// resume. GET /api/v1/threads/{tid}/checkpoints/latest?run_id={rid}. A 404
// (no checkpoint yet) is not an error: found is false, err is nil.
func (c *Client) LatestCheckpoint(ctx context.Context, threadID, runID uuid.UUID) (cp Checkpoint, found bool, err error) {
	path := "/api/v1/threads/" + threadID.String() + "/checkpoints/latest?run_id=" + runID.String()
	var resp checkpointResponse
	status, err := c.doJSON(ctx, http.MethodGet, path, nil, &resp)
	if err != nil {
		if status == http.StatusNotFound {
			return Checkpoint{}, false, nil
		}
		return Checkpoint{}, false, err
	}
	return Checkpoint{ID: resp.CheckpointID, Version: resp.Version, State: []byte(resp.State)}, true, nil
}

// CheckpointByID fetches one checkpoint by its id, scoped to threadID. Used
// when a run was created with RunCreate.checkpoint — the checkpoint it resumes
// from belongs to a PREVIOUS run on the same thread, so it cannot be found by
// this run's own id. GET /api/v1/threads/{tid}/checkpoints/{ckpt}. A 404 (no
// such checkpoint, or one belonging to another thread — the server collapses
// the two) is not an error: found is false, err is nil.
func (c *Client) CheckpointByID(ctx context.Context, threadID uuid.UUID, checkpointID int64) (cp Checkpoint, found bool, err error) {
	path := "/api/v1/threads/" + threadID.String() + "/checkpoints/" + strconv.FormatInt(checkpointID, 10)
	var resp checkpointResponse
	status, err := c.doJSON(ctx, http.MethodGet, path, nil, &resp)
	if err != nil {
		if status == http.StatusNotFound {
			return Checkpoint{}, false, nil
		}
		return Checkpoint{}, false, err
	}
	return Checkpoint{ID: resp.CheckpointID, Version: resp.Version, State: []byte(resp.State)}, true, nil
}

// LoadGraph fetches the GraphDefinition the worker must execute for runID: the
// latest graph registered for the run's assistant. GET
// /api/v1/workers/runs/{rid}/graph. The server returns raw nodes/edges/config
// JSON, which this parses into the worker's own GraphDefinition (keeping the
// worker decoupled from the server's types). A 404 (no graph) surfaces as an
// error — a run with no graph cannot execute.
func (c *Client) LoadGraph(ctx context.Context, runID uuid.UUID) (GraphDefinition, error) {
	var resp workerGraphResponse
	if _, err := c.doJSON(ctx, http.MethodGet, "/api/v1/workers/runs/"+runID.String()+"/graph", nil, &resp); err != nil {
		return GraphDefinition{}, err
	}
	var g GraphDefinition
	if len(resp.Nodes) > 0 {
		if err := json.Unmarshal(resp.Nodes, &g.Nodes); err != nil {
			return GraphDefinition{}, fmt.Errorf("worker: decode graph nodes: %w", err)
		}
	}
	if len(resp.Edges) > 0 {
		if err := json.Unmarshal(resp.Edges, &g.Edges); err != nil {
			return GraphDefinition{}, fmt.Errorf("worker: decode graph edges: %w", err)
		}
	}
	if len(resp.Config) > 0 {
		if err := json.Unmarshal(resp.Config, &g.Config); err != nil {
			return GraphDefinition{}, fmt.Errorf("worker: decode graph config: %w", err)
		}
	}
	return g, nil
}

// eventsPath builds the worker events endpoint path for runID.
func (c *Client) eventsPath(runID uuid.UUID) string {
	return "/api/v1/workers/" + c.workerID.String() + "/runs/" + runID.String() + "/events"
}

// doJSON POSTs/GETs body (marshaled as JSON, unless nil) to path, decodes a
// 2xx response into out (unless nil), and maps errors: 409 -> ErrStaleLease,
// any other non-2xx -> a wrapped error carrying status + body. Returns the
// HTTP status code (0 if the request never got a response) alongside the
// error so callers like LatestCheckpoint can special-case 404 without a
// second round trip.
func (c *Client) doJSON(ctx context.Context, method, path string, body, out any) (int, error) {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, fmt.Errorf("worker: marshal request: %w", err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return 0, fmt.Errorf("worker: build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("worker: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if out != nil && len(respBody) > 0 {
			if err := json.Unmarshal(respBody, out); err != nil {
				return resp.StatusCode, fmt.Errorf("worker: decode response: %w", err)
			}
		}
		return resp.StatusCode, nil
	}
	if resp.StatusCode == http.StatusConflict {
		return resp.StatusCode, ErrStaleLease
	}
	return resp.StatusCode, fmt.Errorf("worker: %s %s: status %d: %s", method, path, resp.StatusCode, string(respBody))
}
