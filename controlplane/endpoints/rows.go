// Hand-written DB row structs + mappers (one set per table, reused by every
// handler that touches that table). Row structs carry `db` tags matching the
// postgres.d2 columns so pgx.RowToStructByName can scan into them; the toAPI
// mappers bridge to the oapi-codegen response types.
//
// NOTE: the OpenAPI response types and the postgres.d2 schema diverge for some
// resources (see DIVERGENCES at the bottom). Mappers map what corresponds and
// leave the rest zero/nil until the API spec ↔ postgres.d2 are reconciled.
package endpoints

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// assistantRow mirrors the assistants table (postgres.d2 workflow_ctx).
type assistantRow struct {
	ID           uuid.UUID `db:"id"`
	GraphID      *string   `db:"graph_id"`
	Name         string    `db:"name"`
	Description  *string   `db:"description"`
	Model        *string   `db:"model"`
	Instructions *string   `db:"instructions"`
	Tools        []byte    `db:"tools"`    // jsonb
	Config       []byte    `db:"config"`   // jsonb
	Metadata     []byte    `db:"metadata"` // jsonb
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
}

// toAPI maps a row to the OpenAPI Assistant response type. config/metadata are
// jsonb in the DB; the API exposes config as a structured object and metadata
// as a free map — both are best-effort unmarshalled here.
func (r assistantRow) toAPI() Assistant {
	a := Assistant{
		AssistantId: r.ID,
		Name:        &r.Name,
		Description: r.Description,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
		Metadata:    map[string]interface{}{},
	}
	if r.GraphID != nil {
		a.GraphId = *r.GraphID
	}
	if len(r.Config) > 0 {
		_ = json.Unmarshal(r.Config, &a.Config)
	}
	if len(r.Metadata) > 0 {
		_ = json.Unmarshal(r.Metadata, &a.Metadata)
	}
	return a
}

// threadRow mirrors the threads table (postgres.d2 workflow_ctx).
type threadRow struct {
	ID        uuid.UUID `db:"id"`
	Status    string    `db:"status"`
	Values    []byte    `db:"values"`   // jsonb, nullable
	Config    []byte    `db:"config"`   // jsonb
	Metadata  []byte    `db:"metadata"` // jsonb
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

// toAPI maps a row to the OpenAPI Thread response type. values/config/metadata
// are jsonb in the DB; the API exposes values/config as optional objects and
// metadata as a free map — best-effort unmarshalled here. interrupts is not
// stored on the thread row (derived from active runs); left nil.
func (r threadRow) toAPI() Thread {
	t := Thread{
		ThreadId:  r.ID,
		Status:    ThreadStatus(r.Status),
		Metadata:  map[string]interface{}{},
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
	if len(r.Values) > 0 {
		var v map[string]interface{}
		_ = json.Unmarshal(r.Values, &v)
		t.Values = &v
	}
	if len(r.Config) > 0 {
		var c map[string]interface{}
		_ = json.Unmarshal(r.Config, &c)
		t.Config = &c
	}
	if len(r.Metadata) > 0 {
		_ = json.Unmarshal(r.Metadata, &t.Metadata)
	}
	return t
}

// runRow mirrors the runs table (postgres.d2 run_ctx).
type runRow struct {
	ID                uuid.UUID  `db:"id"`
	ThreadID          *uuid.UUID `db:"thread_id"` // nullable for stateless runs
	AssistantID       uuid.UUID  `db:"assistant_id"`
	Status            string     `db:"status"`
	Input             []byte     `db:"input"`  // jsonb
	Output            []byte     `db:"output"` // jsonb, nullable
	Error             *string    `db:"error"`
	Metadata          []byte     `db:"metadata"` // jsonb
	Kwargs            []byte     `db:"kwargs"`   // jsonb
	MultitaskStrategy string     `db:"multitask_strategy"`
	Version           int        `db:"version"`
	LeaseEpoch        int        `db:"lease_epoch"`
	WorkerID          *uuid.UUID `db:"worker_id"` // nullable
	Priority          int        `db:"priority"`
	GraphID           *string    `db:"graph_id"` // nullable
	CreatedAt         time.Time  `db:"created_at"`
	StartedAt         *time.Time `db:"started_at"`   // nullable
	CompletedAt       *time.Time `db:"completed_at"` // nullable
	UpdatedAt         time.Time  `db:"updated_at"`
}

// toAPI maps a row to the OpenAPI Run response type. DB status values are
// translated to the API RunStatus enum (queued→pending, in_progress→running,
// requires_action→interrupted, completed→success, failed→error, cancelled→error).
// kwargs/metadata are jsonb, best-effort unmarshalled. DB-only columns
// (input, output, error, version, lease_epoch, worker_id, priority, graph_id,
// started_at, completed_at) are not in the API Run response.
func (r runRow) toAPI() Run {
	run := Run{
		RunId:             r.ID,
		AssistantId:       r.AssistantID,
		Status:            translateRunStatus(r.Status),
		MultitaskStrategy: RunMultitaskStrategy(r.MultitaskStrategy),
		Metadata:          map[string]interface{}{},
		Kwargs:            map[string]interface{}{},
		CreatedAt:         r.CreatedAt,
		UpdatedAt:         r.UpdatedAt,
	}
	if r.ThreadID != nil {
		run.ThreadId = *r.ThreadID
	}
	if len(r.Kwargs) > 0 {
		_ = json.Unmarshal(r.Kwargs, &r.Kwargs)
	}
	if len(r.Metadata) > 0 {
		_ = json.Unmarshal(r.Metadata, &run.Metadata)
	}
	return run
}

// translateRunStatus maps a DB runs.status value to the OpenAPI RunStatus enum.
// DB: queued|in_progress|requires_action|completed|failed|cancelled
// API: pending|running|error|success|timeout|interrupted
func translateRunStatus(dbStatus string) RunStatus {
	switch dbStatus {
	case "queued":
		return RunStatus("pending")
	case "in_progress":
		return RunStatus("running")
	case "requires_action":
		return RunStatus("interrupted")
	case "completed":
		return RunStatus("success")
	case "failed", "cancelled":
		return RunStatus("error")
	default:
		return RunStatus(dbStatus)
	}
}

// cronRow mirrors the crons table (postgres.d2 worker_ctx).
type cronRow struct {
	ID          uuid.UUID  `db:"id"`
	ThreadID    *uuid.UUID `db:"thread_id"` // nullable for stateless crons
	AssistantID uuid.UUID  `db:"assistant_id"`
	Schedule    string     `db:"schedule"`
	Input       []byte     `db:"input"`       // jsonb — API 'payload'
	Config      []byte     `db:"config"`      // jsonb
	Metadata    []byte     `db:"metadata"`    // jsonb
	EndTime     *time.Time `db:"end_time"`    // nullable
	UserID      *string    `db:"user_id"`     // nullable (LangGraph)
	NextRunAt   *time.Time `db:"next_run_at"` // nullable — API 'next_run_date'
	CreatedAt   time.Time  `db:"created_at"`
	UpdatedAt   time.Time  `db:"updated_at"`
}

// toAPI maps a row to the OpenAPI Cron response type. The DB column 'input' is
// exposed as API 'payload'; 'next_run_at' becomes 'next_run_date'. metadata
// is best-effort unmarshalled into the optional map.
func (r cronRow) toAPI() Cron {
	c := Cron{
		CronId:      r.ID,
		AssistantId: &r.AssistantID,
		Schedule:    r.Schedule,
		Payload:     map[string]interface{}{},
		NextRunDate: r.NextRunAt,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
	if r.EndTime != nil {
		c.EndTime = *r.EndTime
	}
	if r.ThreadID != nil {
		c.ThreadId = *r.ThreadID
	}
	if r.UserID != nil {
		c.UserId = r.UserID
	}
	if len(r.Input) > 0 {
		_ = json.Unmarshal(r.Input, &c.Payload)
	}
	if len(r.Metadata) > 0 {
		var m map[string]interface{}
		_ = json.Unmarshal(r.Metadata, &m)
		c.Metadata = &m
	}
	return c
}

// storeItemRow mirrors the store_items table (postgres.d2 store_ctx). namespace
// is TEXT[] (a hierarchical path), value is jsonb. Not event-sourced.
type storeItemRow struct {
	ID        int64     `db:"id"`
	Namespace []string  `db:"namespace"`
	Key       string    `db:"key"`
	Value     []byte    `db:"value"` // jsonb
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

// toAPI maps a store_items row to the OpenAPI Item response type. value is
// jsonb, best-effort unmarshalled into the document map (empty map when null).
func (r storeItemRow) toAPI() Item {
	it := Item{
		Namespace: r.Namespace,
		Key:       r.Key,
		Value:     map[string]interface{}{},
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
	if len(r.Value) > 0 {
		_ = json.Unmarshal(r.Value, &it.Value)
	}
	return it
}

// workerRow mirrors the workers table (postgres.d2 worker_ctx, migration 005).
type workerRow struct {
	WorkerID        uuid.UUID  `db:"worker_id"`
	Graphs          []string   `db:"graphs"`
	Capacity        int        `db:"capacity"`
	ActiveRuns      int        `db:"active_runs"`
	Status          string     `db:"status"`
	LeaseExpiresAt  *time.Time `db:"lease_expires_at"`
	LastHeartbeatAt *time.Time `db:"last_heartbeat_at"`
	CreatedAt       time.Time  `db:"created_at"`
	UpdatedAt       time.Time  `db:"updated_at"`
}

func (r workerRow) toAPI() WorkerRegisterResponse {
	return WorkerRegisterResponse{WorkerID: r.WorkerID, Status: r.Status}
}

// snapshotRow mirrors the snapshots table (postgres.d2 event_sourcing_ctx).
type snapshotRow struct {
	ID          int64     `db:"id"`
	StreamID    uuid.UUID `db:"stream_id"`
	AggregateID uuid.UUID `db:"aggregate_id"`
	Version     int       `db:"version"`
	State       []byte    `db:"state"` // jsonb
	CreatedAt   time.Time `db:"created_at"`
}

func (r snapshotRow) toAPI() CheckpointResponse {
	return CheckpointResponse{
		CheckpointID: r.ID,
		RunID:        r.AggregateID,
		Version:      r.Version,
		State:        json.RawMessage(r.State),
	}
}

// execHistoryRow mirrors execution_history (postgres.d2 run_ctx). Not returned
// over the API in this slice; used by tests to assert node execution.
type execHistoryRow struct {
	ID       int64     `db:"id"`
	RunID    uuid.UUID `db:"run_id"`
	NodeID   string    `db:"node_id"`
	NodeType string    `db:"node_type"`
	Status   string    `db:"status"`
}

// DIVERGENCES (OpenAPI ↔ postgres.d2) — reconcile before tightening mappers:
//   assistants: API has {config(structured), context, version}; DB has
//     {tools, model, instructions, config(jsonb)}. version/context not in DB;
//     tools/model/instructions not in the API Assistant response.
//   threads: API Thread.interrupts is derived from active runs (not a column);
//     ThreadCreate.{thread_id, if_exists, supersteps, ttl} not yet honored by
//     the create impl (always mints a fresh id). ThreadPatch.ttl not honored
//     by the update impl (metadata merge only).
//   runs: API RunStatus {pending,running,error,success,timeout,interrupted} ≠
//     DB status {queued,in_progress,requires_action,completed,failed,cancelled}.
//     cancelled has no API equivalent (mapped to error). API Run has no
//     input/output/error/version/lease_epoch/worker_id/priority/graph_id/
//     started_at/completed_at — all DB-only. RunCreateStateful.{checkpoint,
//     checkpoint_during, command, context, durability, feedback_keys,
//     if_not_exists, interrupt_after/before, on_disconnect, stream_mode,
//     stream_resumable, stream_subgraphs, webhook, after_seconds} not yet
//     honored by the create impl. Stateful create requires assistant_id as
//     UUID but API types it as interface{} (UUID or graph name string).
//   crons: API exposes DB 'input' as 'payload' and 'next_run_at' as
//     'next_run_date'. CronCreate.{config, context, end_time, interrupt_*,
//     metadata, multitask_strategy, webhook, assistant_id as graph-name} not
//     yet honored by create impl. ThreadId is NOT NULL in the API Cron
//     response but nullable in DB for stateless crons (zero UUID when null).
//   store: namespace is TEXT[] (postgres.d2) / []string (OpenAPI Item.Namespace)
//     — the endpoint-queries.d2 `namespace LIKE :prefix` SQL is stale and
//     discarded in favor of array ops. StoreSearchRequest.query (vector/semantic
//     search) is not honored — no vector index in the new control plane yet;
//     filter + namespace_prefix only. StoreListNamespacesRequest.max_depth and
//     .suffix are best-effort (prefix + limit/offset honored).
