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
