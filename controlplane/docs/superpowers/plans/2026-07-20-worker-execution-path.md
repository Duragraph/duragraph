# Worker Execution Path Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the thin-but-reliable worker execution path — push dispatch over NATS with explicit-ack redelivery, worker HTTP endpoints, durable checkpoints, and a 2-step counter worker that proves crash-resume.

**Architecture:** `run-processor` NATS consumer turns `run.created` into a `worker.graph.execute` command on `WORKER_COMMANDS`. A worker (`cmd/worker`) consumes it via a durable pull consumer with explicit ack, leases the run through the epoch-fenced events endpoint, runs a 2-step counter graph writing a checkpoint after each node, and acks only after the run is durably terminal. A mid-run crash leaves the command un-acked → `ack_wait` redelivers → a fresh worker resumes from the last checkpoint (not from zero), fencing the dead worker via `lease_epoch`.

**Tech Stack:** Go 1.25, Echo v4, pgx v5, NATS JetStream (`nats-io/nats.go` jetstream + embedded `nats-server/v2` for tests), testcontainers Postgres.

## Global Constraints

- Source of truth is the **structural** d2 only: `workers.d2`, `nats.d2`, `endpoint-queries.d2` (workers block), `postgres.d2`. The `code-*.d2` files document the OLD `internal/` design — **excluded**.
- Design doc: `controlplane/docs/superpowers/specs/2026-07-20-worker-execution-path-design.md`. It is the contract; this plan implements it.
- **`runs` has NO `lease_expires_at`** — fencing is `lease_epoch` only; JetStream redelivery is the "prior worker dead" trigger.
- Generated `*_gen.go` are never hand-edited — change `endpoints.yaml` / template and regenerate (`go run ./controlplane/gen` from the worktree root `~/worktrees/duragraph/feat/controlplane-worker-execution`). The `custom: true` flag (already in the generator) emits routes-only; hand-written bodies live in a sibling `.go`.
- `execution_history.node_type` CHECK: `start|end|llm|tool|conditional`. `status` CHECK: `started|completed|failed|skipped`. Use only these.
- Worker↔control-plane protocol is internal/native → Go request/response types hand-defined from the d2, NOT added to `duragraph-latest.yaml`.
- Integration tests use testcontainers Postgres + embedded in-process NATS, no build tag, run in standard CI. Reuse the embedded-NATS `TestMain` pattern in `controlplane/nats/nats_integration_test.go`.
- Migrations forward-only; every `*.up.sql` ships a `*.down.sql`.
- Never hand-edit go.mod/go.sum. Conventional commits. Commit after each task. No PR/push/merge without explicit per-PR approval.
- Reuse existing helpers: `writeTx`, `appendEvent`, `Event`, `mustJSON`, `intOr`, `nilIfEmpty`, `Publisher.PublishWithID`, `SubjectFor`, the `custom` generator flag.

---

## File Structure

- `controlplane/db/migrations/tenant/006_worker_checkpoint.{up,down}.sql` — snapshots unique constraint.
- `controlplane/endpoints/worker_types.go` — hand-defined worker protocol types.
- `controlplane/endpoints/rows.go` (modify) — `workerRow`, `snapshotRow`, `execHistoryRow` + mappers.
- `controlplane/endpoints/workers.go` — hand-written worker handlers (register/heartbeat/deregister/events/checkpoints).
- `controlplane/endpoints/workers_gen.go` (regenerate) — routes-only.
- `controlplane/gen/endpoints.yaml` (modify) — mark the 6 worker endpoints `custom`.
- `controlplane/nats/run_processor.go` — dispatch consumer.
- `controlplane/nats/consumers.go` (modify) — add `maxDeliver` to `graph-executor`.
- `controlplane/worker/{client,executor,runner}.go` — worker package.
- `cmd/worker/main.go` — binary.
- `controlplane/server/server.go` (modify) — wire run-processor.
- Tests: `workers_integration_test.go`, `run_processor` test additions, `controlplane/worker/execution_integration_test.go`.
- `spec/models/d2/endpoint-queries.d2` (spec repo) — reconcile.

---

## Task 1: Migration 006 — snapshots unique constraint

**Files:**
- Create: `controlplane/db/migrations/tenant/006_worker_checkpoint.up.sql`
- Create: `controlplane/db/migrations/tenant/006_worker_checkpoint.down.sql`
- Test: `controlplane/endpoints/migration006_test.go`

**Interfaces:**
- Produces: a `UNIQUE (stream_id, version)` constraint named `uq_snapshots_stream_version` on `snapshots`, enabling `ON CONFLICT (stream_id, version)` checkpoint upsert (Task 5).

- [ ] **Step 1: Write the failing test**

Create `controlplane/endpoints/migration006_test.go`:

```go
package endpoints

import (
	"context"
	"testing"
)

func TestSnapshotsUniqueStreamVersion(t *testing.T) {
	ctx := context.Background()
	// A stream to hang snapshots off (FK to event_streams).
	var streamID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO event_streams (stream_id, aggregate_type, aggregate_id, version)
		VALUES (gen_random_uuid(), 'Run', gen_random_uuid(), 0)
		RETURNING stream_id`).Scan(&streamID); err != nil {
		t.Fatal(err)
	}
	ins := `INSERT INTO snapshots (stream_id, aggregate_type, aggregate_id, version, state)
	        VALUES ($1, 'Run', gen_random_uuid(), 1, '{}'::jsonb)`
	if _, err := testPool.Exec(ctx, ins, streamID); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	// Duplicate (stream_id, version) must be rejected by the constraint.
	if _, err := testPool.Exec(ctx, ins, streamID); err == nil {
		t.Fatal("duplicate (stream_id, version) should violate uq_snapshots_stream_version, but insert succeeded")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./controlplane/endpoints/ -run TestSnapshotsUniqueStreamVersion -v`
Expected: FAIL — the second insert succeeds (no constraint yet).

- [ ] **Step 3: Write the migration**

Create `controlplane/db/migrations/tenant/006_worker_checkpoint.up.sql`:

```sql
-- Worker checkpoint idempotency — built from spec/models/d2 (worker execution
-- path). A checkpoint is identified by (stream_id, version); the worker upserts
-- it, and under JetStream redelivery the same (stream_id, version) may be
-- written twice. This constraint makes that upsert idempotent and makes the
-- "latest checkpoint by version" resume lookup unambiguous.
ALTER TABLE snapshots
    ADD CONSTRAINT uq_snapshots_stream_version UNIQUE (stream_id, version);
```

Create `controlplane/db/migrations/tenant/006_worker_checkpoint.down.sql`:

```sql
ALTER TABLE snapshots DROP CONSTRAINT IF EXISTS uq_snapshots_stream_version;
```

- [ ] **Step 4: Run test to verify it passes**

The test harness (`applyTenantMigrations` in `assistants_integration_test.go`) applies every `*.up.sql` in sorted order at `TestMain`, so `006` is picked up automatically.

Run: `go test ./controlplane/endpoints/ -run TestSnapshotsUniqueStreamVersion -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add controlplane/db/migrations/tenant/006_worker_checkpoint.up.sql \
  controlplane/db/migrations/tenant/006_worker_checkpoint.down.sql \
  controlplane/endpoints/migration006_test.go
git commit -m "feat(controlplane): migration 006 — snapshots (stream_id,version) unique for checkpoint upsert"
```

---

## Task 2: Worker protocol types + row structs

**Files:**
- Create: `controlplane/endpoints/worker_types.go`
- Modify: `controlplane/endpoints/rows.go`
- Test: `controlplane/endpoints/worker_rows_test.go`

**Interfaces:**
- Produces (types, consumed by Tasks 3–5 and the worker package via a shared copy):
  - `WorkerRegisterRequest{ WorkerID uuid.UUID; Graphs []string; Capacity int }`
  - `WorkerRegisterResponse{ WorkerID uuid.UUID; Status string }`
  - `WorkerHeartbeatRequest{ Status string; ActiveRuns int }`
  - `WorkerHeartbeatResponse{ Commands []string }`
  - `WorkerEvent{ Type string; LeaseEpoch int; NodeID, NodeType, NodeStatus string; Input, Output json.RawMessage; DurationMs *int; Error *string }`
  - `WorkerEventsRequest{ Events []WorkerEvent }`
  - `RunStartedResponse{ LeaseEpoch int }`
  - `CheckpointWriteRequest{ RunID uuid.UUID; LeaseEpoch, Version int; State json.RawMessage }`
  - `CheckpointWriteResponse{ CheckpointID int64 }`
  - `CheckpointResponse{ CheckpointID int64; RunID uuid.UUID; Version int; State json.RawMessage }`
- Produces (rows): `workerRow`+`toAPI()`, `snapshotRow`+`toAPI()`, `execHistoryRow`.

- [ ] **Step 1: Write the failing test**

Create `controlplane/endpoints/worker_rows_test.go`:

```go
package endpoints

import (
	"testing"
	"time"
)

func TestWorkerRowToAPI(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	r := workerRow{Status: "online", ActiveRuns: 2, Capacity: 4, LeaseExpiresAt: &now}
	got := r.toAPI()
	if got.Status != "online" {
		t.Errorf("status: want online, got %q", got.Status)
	}
}

func TestSnapshotRowToAPI(t *testing.T) {
	r := snapshotRow{ID: 9, AggregateID: mustUUID("11111111-1111-1111-1111-111111111111"), Version: 2, State: []byte(`{"count":2}`)}
	got := r.toAPI()
	if got.CheckpointID != 9 || got.Version != 2 {
		t.Errorf("checkpoint id/version: got %d/%d", got.CheckpointID, got.Version)
	}
	if string(got.State) != `{"count":2}` {
		t.Errorf("state: got %s", got.State)
	}
}
```

Add this helper at the bottom of `worker_rows_test.go` (used above):

```go
func mustUUID(s string) uuid.UUID { u, _ := uuid.Parse(s); return u }
```

and the import `"github.com/google/uuid"`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./controlplane/endpoints/ -run 'TestWorkerRowToAPI|TestSnapshotRowToAPI' -v`
Expected: FAIL — `undefined: workerRow` / `snapshotRow`.

- [ ] **Step 3: Add the protocol types**

Create `controlplane/endpoints/worker_types.go`:

```go
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
```

- [ ] **Step 4: Add the row structs + mappers**

Append to `controlplane/endpoints/rows.go` (before the DIVERGENCES block); `time`, `uuid`, `json` are already imported there:

```go
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
	ID       int64  `db:"id"`
	RunID    uuid.UUID `db:"run_id"`
	NodeID   string `db:"node_id"`
	NodeType string `db:"node_type"`
	Status   string `db:"status"`
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./controlplane/endpoints/ -run 'TestWorkerRowToAPI|TestSnapshotRowToAPI' -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add controlplane/endpoints/worker_types.go controlplane/endpoints/rows.go \
  controlplane/endpoints/worker_rows_test.go
git commit -m "feat(controlplane): worker protocol types + worker/snapshot/exec rows"
```

---

## Task 3: Worker lifecycle endpoints — register / heartbeat / deregister

**Files:**
- Modify: `controlplane/gen/endpoints.yaml`
- Regenerate: `controlplane/endpoints/workers_gen.go`
- Create: `controlplane/endpoints/workers.go`
- Test: `controlplane/endpoints/workers_integration_test.go`

**Interfaces:**
- Consumes: `WorkerRegisterRequest/Response`, `WorkerHeartbeatRequest/Response` (Task 2); `Server.Tenant`; `custom` generator flag.
- Produces: `func (s *Server) RegisterWorkers(g *echo.Group)` (routes); handlers `WorkersRegister`, `WorkersHeartbeat`, `WorkersDeregister`.

- [ ] **Step 1: Write the failing test**

Create `controlplane/endpoints/workers_integration_test.go`:

```go
package endpoints

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

func newTestServerWithWorkers() *echo.Echo {
	e := echo.New()
	s := &Server{Tenant: testPool}
	g := e.Group("/api/v1")
	s.RegisterWorkers(g)
	return e
}

func TestWorkerLifecycle(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE workers, runs, events, outbox, event_streams CASCADE"); err != nil {
		t.Fatal(err)
	}
	e := newTestServerWithWorkers()
	wid := uuid.NewString()

	// register
	if rec := doJSON(t, e, http.MethodPost, "/api/v1/workers/register",
		`{"worker_id":"`+wid+`","graphs":["counter"],"capacity":4}`); rec.Code != http.StatusOK {
		t.Fatalf("register: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var w struct {
		Status   string `json:"status"`
		Capacity int    `json:"capacity"`
	}
	if err := testPool.QueryRow(ctx, `SELECT status, capacity FROM workers WHERE worker_id=$1`, wid).Scan(&w.Status, &w.Capacity); err != nil {
		t.Fatal(err)
	}
	if w.Status != "online" || w.Capacity != 4 {
		t.Errorf("registered worker: got status=%s capacity=%d", w.Status, w.Capacity)
	}

	// heartbeat (lease valid)
	rec := doJSON(t, e, http.MethodPost, "/api/v1/workers/"+wid+"/heartbeat", `{"status":"online","active_runs":1}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("heartbeat: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var hb WorkerHeartbeatResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &hb)
	if hb.Commands == nil {
		t.Error("heartbeat: commands should be non-nil (empty slice)")
	}

	// deregister requeues in-flight runs
	rid := seedInProgressRun(t, ctx, wid) // helper below
	if rec := doJSON(t, e, http.MethodPost, "/api/v1/workers/"+wid+"/deregister", `{}`); rec.Code != http.StatusNoContent {
		t.Fatalf("deregister: want 204, got %d", rec.Code)
	}
	var status string
	var workerID *string
	if err := testPool.QueryRow(ctx, `SELECT status, worker_id::text FROM runs WHERE id=$1`, rid).Scan(&status, &workerID); err != nil {
		t.Fatal(err)
	}
	if status != "queued" || workerID != nil {
		t.Errorf("deregister requeue: want queued/null worker, got %s/%v", status, workerID)
	}
}

// seedInProgressRun inserts an assistant + an in_progress run held by wid.
func seedInProgressRun(t *testing.T, ctx context.Context, wid string) string {
	t.Helper()
	var aid string
	if err := testPool.QueryRow(ctx, `INSERT INTO assistants (name) VALUES ('a') RETURNING id`).Scan(&aid); err != nil {
		t.Fatal(err)
	}
	var rid string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO runs (assistant_id, status, worker_id) VALUES ($1, 'in_progress', $2) RETURNING id`,
		aid, wid).Scan(&rid); err != nil {
		t.Fatal(err)
	}
	return rid
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./controlplane/endpoints/ -run TestWorkerLifecycle -v`
Expected: FAIL — `RegisterWorkers` still generates stub bodies (register won't write the row; heartbeat/deregister return the stub).

- [ ] **Step 3: Mark the lifecycle endpoints custom in the manifest**

In `controlplane/gen/endpoints.yaml`, under `- name: workers`, add `custom: true` to `register`, `heartbeat`, `deregister` (leave `claim`, `stream_events`, `write_checkpoint`, `read_checkpoint` as-is for now — Tasks 4–5 handle events + checkpoints; `claim` stays stub/deferred). Preserve existing fields; only add the `custom: true` line to those three.

- [ ] **Step 4: Regenerate and verify prior groups unchanged**

Run: `go run ./controlplane/gen`
Run: `git diff --stat controlplane/endpoints/assistants_gen.go controlplane/endpoints/store_gen.go`
Expected: empty (other groups byte-identical).
Run: `git diff controlplane/endpoints/workers_gen.go` — `register/heartbeat/deregister` now route-only; the rest keep stub bodies.

- [ ] **Step 5: Write the handlers**

Create `controlplane/endpoints/workers.go`:

```go
// Hand-written worker endpoints (worker↔control-plane protocol). Routes are
// generated into workers_gen.go (endpoints marked custom in endpoints.yaml);
// bodies live here. Source of truth: spec/models/d2 workers block + workers.d2
// + the worker-execution design doc. runs has NO lease_expires_at — fencing is
// lease_epoch only.
package endpoints

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

// WorkersRegister upserts a worker as online. POST /workers/register -> 200.
func (s *Server) WorkersRegister(c echo.Context) error {
	ctx := c.Request().Context()
	var req WorkerRegisterRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if _, err := s.Tenant.Exec(ctx, `
		INSERT INTO workers (worker_id, graphs, capacity, status, lease_expires_at, last_heartbeat_at)
		VALUES ($1, $2, $3, 'online', now() + interval '60 seconds', now())
		ON CONFLICT (worker_id) DO UPDATE
		  SET graphs=EXCLUDED.graphs, capacity=EXCLUDED.capacity, status='online',
		      lease_expires_at=now() + interval '60 seconds', last_heartbeat_at=now()`,
		req.WorkerID, req.Graphs, req.Capacity); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, WorkerRegisterResponse{WorkerID: req.WorkerID, Status: "online"})
}

// WorkersHeartbeat renews the worker lease. POST /workers/{id}/heartbeat -> 200.
func (s *Server) WorkersHeartbeat(c echo.Context) error {
	ctx := c.Request().Context()
	wid := c.Param("id")
	var req WorkerHeartbeatRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	ct, err := s.Tenant.Exec(ctx, `
		UPDATE workers SET status=$2, active_runs=$3,
		  lease_expires_at=now() + interval '60 seconds', last_heartbeat_at=now()
		WHERE worker_id=$1 AND lease_expires_at > now()`,
		wid, req.Status, req.ActiveRuns)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if ct.RowsAffected() == 0 {
		// Unknown worker or expired lease — worker must re-register.
		return echo.NewHTTPError(http.StatusConflict, "worker lease expired or unknown; re-register")
	}
	return c.JSON(http.StatusOK, WorkerHeartbeatResponse{Commands: []string{}})
}

// WorkersDeregister marks the worker offline and requeues its in-flight runs.
// POST /workers/{id}/deregister -> 204.
func (s *Server) WorkersDeregister(c echo.Context) error {
	ctx := c.Request().Context()
	wid := c.Param("id")
	tx, err := s.Tenant.Begin(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `UPDATE workers SET status='offline' WHERE worker_id=$1`, wid); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if _, err := tx.Exec(ctx, `
		UPDATE runs SET status='queued', worker_id=NULL
		WHERE worker_id=$1 AND status='in_progress'`, wid); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if err := tx.Commit(ctx); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

var _ = errors.Is // retained for Tasks 4–5 handlers added to this file
var _ = pgx.ErrNoRows
```

Note: remove the two trailing `var _ =` lines in Step 3 of Task 4 once the real uses land — they exist only so this file compiles standalone in this task. (If `go vet` complains now, keep them; they are removed when events/checkpoints add real `errors`/`pgx` uses.)

- [ ] **Step 6: Build, run the test**

Run: `go build ./controlplane/...`
Run: `go test ./controlplane/endpoints/ -run TestWorkerLifecycle -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add controlplane/gen/endpoints.yaml controlplane/endpoints/workers_gen.go \
  controlplane/endpoints/workers.go controlplane/endpoints/workers_integration_test.go
git commit -m "feat(controlplane): worker register/heartbeat/deregister endpoints"
```

---

## Task 4: Events endpoint — run lifecycle + node events, epoch-fenced

**Files:**
- Modify: `controlplane/gen/endpoints.yaml` (mark `stream_events` custom)
- Regenerate: `controlplane/endpoints/workers_gen.go`
- Modify: `controlplane/endpoints/workers.go` (add `WorkersStreamEvents`)
- Test: `controlplane/endpoints/workers_integration_test.go` (add cases)

**Interfaces:**
- Consumes: `WorkerEventsRequest`, `WorkerEvent`, `RunStartedResponse` (Task 2); `writeTx`, `Event`, `mustJSON`.
- Produces: `func (s *Server) WorkersStreamEvents(c echo.Context) error` handling `POST /workers/{id}/runs/{rid}/events`. run.started returns `RunStartedResponse{lease_epoch}`; node/terminal events are epoch-fenced (409 on mismatch/terminal).

- [ ] **Step 1: Write the failing test**

Add to `controlplane/endpoints/workers_integration_test.go`:

```go
func TestWorkerEventsLifecycle(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE workers, runs, execution_history, events, outbox, event_streams, assistants CASCADE"); err != nil {
		t.Fatal(err)
	}
	e := newTestServerWithWorkers()
	wid := uuid.NewString()
	var aid, rid string
	_ = testPool.QueryRow(ctx, `INSERT INTO assistants (name) VALUES ('a') RETURNING id`).Scan(&aid)
	_ = testPool.QueryRow(ctx, `INSERT INTO runs (assistant_id, status) VALUES ($1,'queued') RETURNING id`, aid).Scan(&rid)

	base := "/api/v1/workers/" + wid + "/runs/" + rid + "/events"

	// run.started leases the run and returns epoch 1
	rec := doJSON(t, e, http.MethodPost, base, `{"events":[{"type":"run.started"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("run.started: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var rs RunStartedResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &rs)
	if rs.LeaseEpoch != 1 {
		t.Fatalf("run.started epoch: want 1, got %d", rs.LeaseEpoch)
	}
	var st string
	var le int
	_ = testPool.QueryRow(ctx, `SELECT status, lease_epoch FROM runs WHERE id=$1`, rid).Scan(&st, &le)
	if st != "in_progress" || le != 1 {
		t.Errorf("after start: want in_progress/epoch1, got %s/%d", st, le)
	}

	// node_completed with correct epoch -> execution_history row + 200
	rec = doJSON(t, e, http.MethodPost, base,
		`{"events":[{"type":"execution.node_completed","lease_epoch":1,"node_id":"A","node_type":"tool","node_status":"completed"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("node event: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var n int
	_ = testPool.QueryRow(ctx, `SELECT count(*) FROM execution_history WHERE run_id=$1`, rid).Scan(&n)
	if n != 1 {
		t.Errorf("execution_history: want 1, got %d", n)
	}

	// stale epoch -> 409
	rec = doJSON(t, e, http.MethodPost, base,
		`{"events":[{"type":"execution.node_completed","lease_epoch":0,"node_id":"A","node_type":"tool","node_status":"completed"}]}`)
	if rec.Code != http.StatusConflict {
		t.Errorf("stale epoch: want 409, got %d", rec.Code)
	}

	// run.completed -> completed
	rec = doJSON(t, e, http.MethodPost, base, `{"events":[{"type":"run.completed","lease_epoch":1}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("run.completed: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	_ = testPool.QueryRow(ctx, `SELECT status FROM runs WHERE id=$1`, rid).Scan(&st)
	if st != "completed" {
		t.Errorf("after complete: want completed, got %s", st)
	}

	// run.started on a terminal run -> 409
	rec = doJSON(t, e, http.MethodPost, base, `{"events":[{"type":"run.started"}]}`)
	if rec.Code != http.StatusConflict {
		t.Errorf("start on terminal: want 409, got %d", rec.Code)
	}

	// outbox carries run.started + node_completed + run.completed
	var oc int
	_ = testPool.QueryRow(ctx, `SELECT count(*) FROM outbox WHERE aggregate_id=$1`, rid).Scan(&oc)
	if oc < 3 {
		t.Errorf("outbox events: want >=3, got %d", oc)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./controlplane/endpoints/ -run TestWorkerEventsLifecycle -v`
Expected: FAIL — `stream_events` still returns the stub.

- [ ] **Step 3: Mark `stream_events` custom + regenerate**

In `controlplane/gen/endpoints.yaml`, add `custom: true` to the workers `stream_events` endpoint. Run `go run ./controlplane/gen`. Confirm other groups unchanged (`git diff --stat` empty for them) and `workers_gen.go` now routes `WorkersStreamEvents`.

Also remove the two placeholder `var _ =` lines from `workers.go` (Task 3 Step 5) — this task's handler uses `errors` and `pgx` for real.

- [ ] **Step 4: Add the events handler to `workers.go`**

Append to `controlplane/endpoints/workers.go`:

```go
// WorkersStreamEvents is the single worker→server state channel.
// POST /workers/{id}/runs/{rid}/events. run.started establishes the lease and
// returns the epoch; all other events are fenced on lease_epoch. Source: design
// doc "events endpoint". runs has no lease_expires_at — fence on epoch only.
func (s *Server) WorkersStreamEvents(c echo.Context) error {
	ctx := c.Request().Context()
	wid := c.Param("id")
	rid := c.Param("rid")
	var req WorkerEventsRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	var startedEpoch *int
	for _, ev := range req.Events {
		switch ev.Type {
		case "run.started":
			epoch, err := s.leaseRun(ctx, rid, wid)
			if err != nil {
				return err // already-typed echo error (409/500)
			}
			startedEpoch = &epoch
		case "run.completed", "run.failed":
			if err := s.terminalRun(ctx, rid, ev); err != nil {
				return err
			}
		case "execution.node_started", "execution.node_completed", "execution.node_failed":
			if err := s.nodeEvent(ctx, rid, ev); err != nil {
				return err
			}
		default:
			return echo.NewHTTPError(http.StatusBadRequest, "unknown event type: "+ev.Type)
		}
	}
	if startedEpoch != nil {
		return c.JSON(http.StatusOK, RunStartedResponse{LeaseEpoch: *startedEpoch})
	}
	return c.NoContent(http.StatusOK)
}

// leaseRun re-leases a non-terminal run to wid, bumping lease_epoch, and emits
// run.started. Returns the new epoch. 409 if the run is already terminal.
func (s *Server) leaseRun(ctx context.Context, rid, wid string) (int, error) {
	var epoch int
	err := s.writeTx(ctx, s.Tenant, []Event{{AggregateType: "Run", AggregateID: mustParseUUID(rid), EventType: "run.started"}},
		func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `
				UPDATE runs SET status='in_progress', worker_id=$2,
				  lease_epoch=lease_epoch+1, started_at=COALESCE(started_at, now())
				WHERE id=$1 AND status IN ('queued','in_progress')
				RETURNING lease_epoch`, rid, wid).Scan(&epoch)
		})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, echo.NewHTTPError(http.StatusConflict, "run is terminal or not found")
		}
		return 0, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return epoch, nil
}

// errStaleLease is returned from a projection when the epoch guard rejects the
// write (0 rows affected). writeTxOrHTTP maps it to 409. Because writeTx appends
// the event BEFORE the projection runs, returning this rolls the whole tx back —
// so a stale worker's event is never committed.
var errStaleLease = errors.New("stale lease_epoch")

// nodeEvent records a node execution row + emits the execution.node_* event.
// The epoch guard is ATOMIC (conditional INSERT), not a separate read.
func (s *Server) nodeEvent(ctx context.Context, rid string, ev WorkerEvent) error {
	return s.writeTxOrHTTP(ctx, rid, ev.Type, ev, func(tx pgx.Tx) error {
		ct, err := tx.Exec(ctx, `
			INSERT INTO execution_history (run_id, node_id, node_type, status, input, output, error, duration_ms, completed_at)
			SELECT $1,$2,$3,$4,$5,$6,$7,$8, CASE WHEN $4 <> 'started' THEN now() END
			WHERE EXISTS (SELECT 1 FROM runs WHERE id=$1 AND lease_epoch=$9)`,
			rid, ev.NodeID, ev.NodeType, ev.NodeStatus,
			jsonOrEmpty(ev.Input), jsonOrEmpty(ev.Output), ev.Error, ev.DurationMs, ev.LeaseEpoch)
		if err != nil {
			return err
		}
		if ct.RowsAffected() == 0 {
			return errStaleLease
		}
		return nil
	})
}

// terminalRun marks the run completed/failed + emits the event. Epoch guarded
// IN the UPDATE (WHERE lease_epoch=$epoch); 0 rows → stale → 409.
func (s *Server) terminalRun(ctx context.Context, rid string, ev WorkerEvent) error {
	status := "completed"
	if ev.Type == "run.failed" {
		status = "failed"
	}
	return s.writeTxOrHTTP(ctx, rid, ev.Type, ev, func(tx pgx.Tx) error {
		ct, err := tx.Exec(ctx,
			`UPDATE runs SET status=$2, completed_at=now(), error=$3 WHERE id=$1 AND lease_epoch=$4`,
			rid, status, ev.Error, ev.LeaseEpoch)
		if err != nil {
			return err
		}
		if ct.RowsAffected() == 0 {
			return errStaleLease
		}
		return nil
	})
}

// writeTxOrHTTP runs a projection inside writeTx (emitting one event) and maps
// errStaleLease → 409, other errors → 500.
func (s *Server) writeTxOrHTTP(ctx context.Context, rid, eventType string, ev WorkerEvent, proj func(pgx.Tx) error) error {
	events := []Event{{AggregateType: "Run", AggregateID: mustParseUUID(rid), EventType: eventType, Payload: mustJSON(ev)}}
	if err := s.writeTx(ctx, s.Tenant, events, proj); err != nil {
		if errors.Is(err, errStaleLease) {
			return echo.NewHTTPError(http.StatusConflict, "stale lease_epoch")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return nil
}
```

Add to `controlplane/endpoints/runtime.go` a UUID parse helper (used above and in Task 5):

```go
// mustParseUUID parses a UUID string, returning the zero UUID on failure (the
// caller has already validated the path param, or the DB FK will reject zero).
func mustParseUUID(s string) uuid.UUID { u, _ := uuid.Parse(s); return u }
```

(`uuid` is already imported in runtime.go.)

- [ ] **Step 5: Build, run the test**

Run: `go build ./controlplane/...`
Run: `go test ./controlplane/endpoints/ -run 'TestWorkerEventsLifecycle|TestWorkerLifecycle' -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add controlplane/gen/endpoints.yaml controlplane/endpoints/workers_gen.go \
  controlplane/endpoints/workers.go controlplane/endpoints/runtime.go \
  controlplane/endpoints/workers_integration_test.go
git commit -m "feat(controlplane): worker events endpoint — run lifecycle + node events, epoch-fenced"
```

---

## Task 5: Checkpoint endpoints — write (epoch-fenced upsert) + read + resume lookup

**Files:**
- Modify: `controlplane/gen/endpoints.yaml` (mark `write_checkpoint`, `read_checkpoint` custom)
- Regenerate: `controlplane/endpoints/workers_gen.go`
- Modify: `controlplane/endpoints/workers.go` (add checkpoint handlers)
- Test: `controlplane/endpoints/workers_integration_test.go` (add cases)

**Interfaces:**
- Consumes: `CheckpointWriteRequest/Response`, `CheckpointResponse` (Task 2); `snapshotRow` (Task 2); the `uq_snapshots_stream_version` constraint (Task 1); `errStaleLease` (Task 4).
- Produces: `WorkersWriteCheckpoint` (`POST /threads/{tid}/checkpoints`), `WorkersReadCheckpoint` (`GET /threads/{tid}/checkpoints/{ckpt}`), and a latest-for-run lookup exposed for the worker resume (query param `?run_id=&latest=true` on read, or a dedicated handler — implement as documented below).

- [ ] **Step 1: Write the failing test**

Add to `controlplane/endpoints/workers_integration_test.go`:

```go
func TestWorkerCheckpoints(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE workers, runs, snapshots, events, outbox, event_streams, assistants, threads CASCADE"); err != nil {
		t.Fatal(err)
	}
	e := newTestServerWithWorkers()
	wid := uuid.NewString()
	var tid, aid, rid string
	_ = testPool.QueryRow(ctx, `INSERT INTO threads DEFAULT VALUES RETURNING id`).Scan(&tid)
	_ = testPool.QueryRow(ctx, `INSERT INTO assistants (name) VALUES ('a') RETURNING id`).Scan(&aid)
	_ = testPool.QueryRow(ctx, `INSERT INTO runs (thread_id, assistant_id, status) VALUES ($1,$2,'queued') RETURNING id`, tid, aid).Scan(&rid)

	// lease the run first (creates the event_stream that snapshots FK-references)
	started := doJSON(t, e, http.MethodPost, "/api/v1/workers/"+wid+"/runs/"+rid+"/events", `{"events":[{"type":"run.started"}]}`)
	var rs RunStartedResponse
	_ = json.Unmarshal(started.Body.Bytes(), &rs)

	cp := "/api/v1/threads/" + tid + "/checkpoints"
	body := func(v int, st string) string {
		return `{"run_id":"` + rid + `","lease_epoch":` + itoa(rs.LeaseEpoch) + `,"version":` + itoa(v) + `,"state":` + st + `}`
	}

	// write checkpoint v1
	if rec := doJSON(t, e, http.MethodPost, cp, body(1, `{"count":1}`)); rec.Code != http.StatusOK {
		t.Fatalf("write v1: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	// upsert v1 (same version) is idempotent -> still 200, still one row
	if rec := doJSON(t, e, http.MethodPost, cp, body(1, `{"count":1}`)); rec.Code != http.StatusOK {
		t.Fatalf("upsert v1: want 200, got %d", rec.Code)
	}
	var cnt int
	_ = testPool.QueryRow(ctx, `SELECT count(*) FROM snapshots WHERE aggregate_id=$1`, rid).Scan(&cnt)
	if cnt != 1 {
		t.Errorf("after upsert: want 1 snapshot, got %d", cnt)
	}
	// write v2
	_ = doJSON(t, e, http.MethodPost, cp, body(2, `{"count":2}`))

	// stale epoch -> 409
	if rec := doJSON(t, e, http.MethodPost, cp, `{"run_id":"`+rid+`","lease_epoch":999,"version":3,"state":{}}`); rec.Code != http.StatusConflict {
		t.Errorf("stale checkpoint: want 409, got %d", rec.Code)
	}

	// resume lookup: latest checkpoint for run -> version 2
	rec := doJSON(t, e, http.MethodGet, cp+"/latest?run_id="+rid, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("latest: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got CheckpointResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Version != 2 {
		t.Errorf("latest version: want 2, got %d", got.Version)
	}
}
```

Add this helper (once) to the test file, with `import "strconv"`:

```go
func itoa(n int) string { return strconv.Itoa(n) }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./controlplane/endpoints/ -run TestWorkerCheckpoints -v`
Expected: FAIL — checkpoint endpoints still stub.

- [ ] **Step 3: Mark checkpoints custom + add a `latest` route**

In `controlplane/gen/endpoints.yaml`, add `custom: true` to `write_checkpoint` and `read_checkpoint`. Add a new endpoint entry to the workers group for the resume lookup:

```yaml
  - name: latest_checkpoint
    method: GET
    path: /threads/{tid}/checkpoints/latest
    kind: read
    outbox: false
    custom: true
```

Run `go run ./controlplane/gen`. Confirm other groups unchanged; `workers_gen.go` now routes `WorkersWriteCheckpoint`, `WorkersReadCheckpoint`, `WorkersLatestCheckpoint`. (Register order: ensure `/checkpoints/latest` is registered — Echo matches static `/latest` before the `/:ckpt` param when both exist; if a conflict arises, the `latest` handler guards on the `run_id` query param.)

- [ ] **Step 4: Add checkpoint handlers to `workers.go`**

Append to `controlplane/endpoints/workers.go`:

```go
// WorkersWriteCheckpoint upserts a run snapshot, epoch-fenced.
// POST /threads/{tid}/checkpoints -> 200 {checkpoint_id}.
func (s *Server) WorkersWriteCheckpoint(c echo.Context) error {
	ctx := c.Request().Context()
	var req CheckpointWriteRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	rid := req.RunID.String()
	// Resolve the run's event stream (created by run.started's writeTx).
	var streamID string
	if err := s.Tenant.QueryRow(ctx,
		`SELECT stream_id FROM event_streams WHERE aggregate_type='Run' AND aggregate_id=$1`, rid).Scan(&streamID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusConflict, "run stream not initialized (post run.started first)")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	// Atomic epoch guard: the INSERT..SELECT only fires when the run's current
	// lease_epoch matches; a stale worker inserts nothing → 0 rows → 409.
	var id int64
	err := s.Tenant.QueryRow(ctx, `
		INSERT INTO snapshots (stream_id, aggregate_type, aggregate_id, version, state)
		SELECT $1, 'Run', $2, $3, $4
		WHERE EXISTS (SELECT 1 FROM runs WHERE id=$2 AND lease_epoch=$5)
		ON CONFLICT (stream_id, version) DO UPDATE SET state=EXCLUDED.state
		RETURNING id`,
		streamID, rid, req.Version, jsonOrEmpty(req.State), req.LeaseEpoch).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusConflict, "stale lease_epoch")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, CheckpointWriteResponse{CheckpointID: id})
}

// WorkersReadCheckpoint returns one snapshot by id, scoped to the thread's runs.
// GET /threads/{tid}/checkpoints/{ckpt} -> 200 / 404.
func (s *Server) WorkersReadCheckpoint(c echo.Context) error {
	ctx := c.Request().Context()
	tid := c.Param("tid")
	ckpt := c.Param("ckpt")
	rows, err := s.Tenant.Query(ctx, `
		SELECT id, stream_id, aggregate_id, version, state, created_at
		FROM snapshots
		WHERE id=$1 AND aggregate_id IN (SELECT id FROM runs WHERE thread_id=$2)`, ckpt, tid)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	row, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[snapshotRow])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, row.toAPI())
}

// WorkersLatestCheckpoint returns the highest-version snapshot for a run, for
// worker resume. GET /threads/{tid}/checkpoints/latest?run_id=... -> 200 / 404.
func (s *Server) WorkersLatestCheckpoint(c echo.Context) error {
	ctx := c.Request().Context()
	rid := c.QueryParam("run_id")
	if rid == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "run_id is required")
	}
	rows, err := s.Tenant.Query(ctx, `
		SELECT id, stream_id, aggregate_id, version, state, created_at
		FROM snapshots WHERE aggregate_id=$1 ORDER BY version DESC LIMIT 1`, rid)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	row, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[snapshotRow])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "no checkpoint")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, row.toAPI())
}
```

- [ ] **Step 5: Build, run the tests**

Run: `go build ./controlplane/...`
Run: `go test ./controlplane/endpoints/ -run TestWorkerCheckpoints -v`
Expected: PASS.

Run: `go test ./controlplane/endpoints/` (full package regression)
Expected: PASS (store/system/assistants/etc. still green).

- [ ] **Step 6: Commit**

```bash
git add controlplane/gen/endpoints.yaml controlplane/endpoints/workers_gen.go \
  controlplane/endpoints/workers.go controlplane/endpoints/workers_integration_test.go
git commit -m "feat(controlplane): checkpoint write (epoch-fenced upsert) + read + latest-for-run resume lookup"
```

---

## Task 6: run-processor dispatch consumer + max_deliver on graph-executor

**Files:**
- Create: `controlplane/nats/run_processor.go`
- Modify: `controlplane/nats/consumers.go` (add `maxDeliver` to `graph-executor`)
- Test: `controlplane/nats/run_processor_test.go`

**Interfaces:**
- Consumes: `jetstream.JetStream`, `Publisher.PublishWithID`, the `RUNS` stream + `WORKER_COMMANDS` stream (already ensured), the `run-processor` consumer spec (already declared).
- Produces: `type RunProcessor` with `NewRunProcessor(js jetstream.JetStream, pub *Publisher) *RunProcessor` and `func (rp *RunProcessor) Start(ctx context.Context) error` (blocks, consuming `run-processor` until ctx cancels) + `Stop()`. Publishes `duragraph.worker_commands.worker.graph.execute` with `Nats-Msg-Id = run_id` and payload `{run_id, thread_id, assistant_id, graph_id, input}`.

- [ ] **Step 1: Write the failing test**

Create `controlplane/nats/run_processor_test.go` (package `nats_test`, reuses the embedded-NATS `TestMain` + `natsURL`):

```go
package nats_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	dnats "github.com/duragraph/duragraph/controlplane/nats"
	"github.com/nats-io/nats.go/jetstream"
)

func TestRunProcessorDispatch(t *testing.T) {
	ctx := context.Background()
	nc, js := connectJS(t) // helper in this package's test files; see nats_integration_test.go
	defer nc.Close()
	if err := dnats.EnsureStreams(ctx, js); err != nil {
		t.Fatal(err)
	}
	if err := dnats.EnsureConsumers(ctx, js); err != nil {
		t.Fatal(err)
	}

	rp := dnats.NewRunProcessor(js, dnats.NewPublisher(js))
	go func() { _ = rp.Start(ctx) }()
	defer rp.Stop()

	// Subscribe to WORKER_COMMANDS to observe the dispatched command.
	cons, err := js.CreateOrUpdateConsumer(ctx, "WORKER_COMMANDS", jetstream.ConsumerConfig{
		FilterSubject: "duragraph.worker_commands.worker.graph.execute",
		AckPolicy:     jetstream.AckExplicitPolicy,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Publish a run.created onto RUNS (Nats-Msg-Id = run_id).
	pub := dnats.NewPublisher(js)
	runID := "11111111-1111-1111-1111-111111111111"
	payload := map[string]any{"run_id": runID, "assistant_id": "22222222-2222-2222-2222-222222222222", "graph_id": "counter"}
	if err := pub.PublishWithID(ctx, dnats.SubjectFor("run.created"), runID, payload); err != nil {
		t.Fatal(err)
	}
	// Publish the SAME run.created again → dedup → run-processor sees one.
	_ = pub.PublishWithID(ctx, dnats.SubjectFor("run.created"), runID, payload)

	batch, err := cons.Fetch(1, jetstream.FetchMaxWait(5*time.Second))
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	var got jetstream.Msg
	for m := range batch.Messages() {
		got = m
	}
	if got == nil {
		t.Fatal("no worker.graph.execute command dispatched")
	}
	var cmd map[string]any
	_ = json.Unmarshal(got.Data(), &cmd)
	if cmd["run_id"] != runID {
		t.Errorf("command run_id: want %s, got %v", runID, cmd["run_id"])
	}
	_ = got.Ack()

	// No second command (RUNS dedup on Nats-Msg-Id → single command).
	batch2, err := cons.Fetch(1, jetstream.FetchMaxWait(1*time.Second))
	if err != nil {
		t.Fatalf("fetch2: %v", err)
	}
	n := 0
	for range batch2.Messages() {
		n++
	}
	if n != 0 {
		t.Errorf("expected only one dispatched command (dedup), got a second")
	}
}
```

If a `connectJS(t)` helper does not already exist in the package's test files, add one mirroring the connection code in `nats_integration_test.go` (dial `natsURL`, return `*nats.Conn` + `jetstream.JetStream`).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./controlplane/nats/ -run TestRunProcessorDispatch -v`
Expected: FAIL — `undefined: NewRunProcessor`.

- [ ] **Step 3: Add max_deliver to the graph-executor consumer**

In `controlplane/nats/consumers.go`, add a `maxDeliver` field to the `graph-executor` spec so poison/persistently-failing commands dead-letter instead of redelivering forever. Change its `tenantConsumers` entry to include `maxDeliver: 5`:

```go
{name: "graph-executor", stream: "WORKER_COMMANDS", filter: "duragraph.worker_commands.worker.graph.execute", ackWait: 5 * time.Minute, maxDeliver: 5},
```

(The `consumerSpec` struct already has `maxDeliver` and `EnsureConsumers` already applies it.)

- [ ] **Step 4: Write the run-processor**

Create `controlplane/nats/run_processor.go`:

```go
// run-processor: the dispatch consumer. Drains the RUNS stream (filter
// run.created) and turns each queued run into a worker.graph.execute command on
// WORKER_COMMANDS, with Nats-Msg-Id = run_id so a duplicate run.created yields a
// single command. Thin dispatcher — it does not mutate run state; the worker
// leases via run.started. Source: spec/models/d2/nats.d2 + endpoint-queries.d2.
package nats

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go/jetstream"
)

type RunProcessor struct {
	js   jetstream.JetStream
	pub  *Publisher
	stop context.CancelFunc
}

func NewRunProcessor(js jetstream.JetStream, pub *Publisher) *RunProcessor {
	return &RunProcessor{js: js, pub: pub}
}

// runCreatedPayload is the subset of the run.created event the dispatcher needs.
type runCreatedPayload struct {
	RunID       string          `json:"run_id"`
	ThreadID    string          `json:"thread_id,omitempty"`
	AssistantID string          `json:"assistant_id"`
	GraphID     string          `json:"graph_id,omitempty"`
	Input       json.RawMessage `json:"input,omitempty"`
}

// Start binds the durable run-processor consumer and blocks, dispatching each
// run.created as a worker.graph.execute command until ctx is cancelled.
func (rp *RunProcessor) Start(ctx context.Context) error {
	ctx, rp.stop = context.WithCancel(ctx)
	cons, err := rp.js.Consumer(ctx, "RUNS", "run-processor")
	if err != nil {
		return fmt.Errorf("run-processor: bind consumer: %w", err)
	}
	cc, err := cons.Consume(func(msg jetstream.Msg) {
		if err := rp.dispatch(ctx, msg); err != nil {
			_ = msg.Nak() // transient: let it redeliver
			return
		}
		_ = msg.Ack()
	})
	if err != nil {
		return fmt.Errorf("run-processor: consume: %w", err)
	}
	defer cc.Stop()
	<-ctx.Done()
	return nil
}

func (rp *RunProcessor) dispatch(ctx context.Context, msg jetstream.Msg) error {
	// Only run.created triggers dispatch; ignore other run.* subjects on RUNS.
	if !hasSuffix(msg.Subject(), "run.created") {
		return nil
	}
	var p runCreatedPayload
	if err := json.Unmarshal(msg.Data(), &p); err != nil {
		return nil // malformed → drop (ack); nothing to retry
	}
	if p.RunID == "" {
		return nil
	}
	cmd := map[string]any{
		"run_id": p.RunID, "thread_id": p.ThreadID,
		"assistant_id": p.AssistantID, "graph_id": p.GraphID, "input": p.Input,
	}
	// Nats-Msg-Id = run_id → one command per run even if run.created duplicates.
	return rp.pub.PublishWithID(ctx, SubjectFor("worker.graph.execute"), p.RunID, cmd)
}

func (rp *RunProcessor) Stop() {
	if rp.stop != nil {
		rp.stop()
	}
}

func hasSuffix(s, suf string) bool {
	return len(s) >= len(suf) && s[len(s)-len(suf):] == suf
}
```

Note: verify `SubjectFor("worker.graph.execute")` yields `duragraph.worker_commands.worker.graph.execute` — check `categoryFor` in `publisher.go` maps the `worker.` prefix to the `worker_commands` category. If it does not, extend `categoryFor` to map `worker` → `worker_commands` (small, covered by the test's subscription filter).

- [ ] **Step 5: Build, run the test**

Run: `go build ./controlplane/...`
Run: `go test ./controlplane/nats/ -run TestRunProcessorDispatch -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add controlplane/nats/run_processor.go controlplane/nats/consumers.go \
  controlplane/nats/run_processor_test.go
git commit -m "feat(controlplane): run-processor dispatch consumer + graph-executor max_deliver"
```

---

## Task 7: Worker package — HTTP client + 2-step counter executor

**Files:**
- Create: `controlplane/worker/client.go`
- Create: `controlplane/worker/executor.go`
- Test: `controlplane/worker/client_test.go`

**Interfaces:**
- Produces:
  - `type Client` with `NewClient(baseURL string, workerID uuid.UUID, http *http.Client) *Client` and methods `Register(ctx, graphs []string, capacity int) error`, `Heartbeat(ctx, activeRuns int) error`, `Deregister(ctx) error`, `RunStarted(ctx, runID uuid.UUID) (epoch int, err error)`, `NodeCompleted(ctx, runID uuid.UUID, epoch int, nodeID, nodeType string) error`, `RunCompleted(ctx, runID uuid.UUID, epoch int) error`, `RunFailed(ctx, runID uuid.UUID, epoch int, reason string) error`, `WriteCheckpoint(ctx, threadID, runID uuid.UUID, epoch, version int, state []byte) error`, `LatestCheckpoint(ctx, threadID, runID uuid.UUID) (version int, state []byte, found bool, err error)`. A 409 from any epoch-fenced call returns a sentinel `ErrStaleLease`.
  - `type CounterExecutor` implementing the 2-step counter graph: `func (CounterExecutor) Nodes() []string` → `["A","B"]`; `func (CounterExecutor) Run(step int, state map[string]int) map[string]int` (A→count=1, B→count=2).
- Consumes: the worker endpoints from Tasks 3–5 (over HTTP).

- [ ] **Step 1: Write the failing test**

Create `controlplane/worker/client_test.go`. It mounts the real endpoints over `httptest` against the shared `testPool`, so it exercises the actual handlers. (This test package imports `controlplane/endpoints`; use a small `TestMain` that stands up a Postgres testcontainer + migrations, mirroring `endpoints`'s harness — or, to avoid duplication, keep this test in `package worker_test` and start an `httptest.Server` wrapping an `echo.Echo` with the worker routes and a pool.)

```go
package worker_test

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/duragraph/duragraph/controlplane/endpoints"
	"github.com/duragraph/duragraph/controlplane/worker"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

func TestClientLifecycleAndCheckpoints(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t) // helper: testcontainer PG + tenant migrations (see execution_integration_test.go)
	e := echo.New()
	g := e.Group("/api/v1")
	(&endpoints.Server{Tenant: pool}).RegisterWorkers(g)
	srv := httptest.NewServer(e)
	defer srv.Close()

	wid := uuid.New()
	cl := worker.NewClient(srv.URL, wid, srv.Client())

	if err := cl.Register(ctx, []string{"counter"}, 1); err != nil {
		t.Fatal(err)
	}
	tid, aid, rid := seedThreadAssistantRun(t, ctx, pool) // queued run on a thread
	epoch, err := cl.RunStarted(ctx, rid)
	if err != nil || epoch != 1 {
		t.Fatalf("run started: epoch=%d err=%v", epoch, err)
	}
	if err := cl.WriteCheckpoint(ctx, tid, rid, epoch, 1, []byte(`{"count":1}`)); err != nil {
		t.Fatal(err)
	}
	v, state, found, err := cl.LatestCheckpoint(ctx, tid, rid)
	if err != nil || !found || v != 1 || string(state) != `{"count":1}` {
		t.Fatalf("latest: v=%d found=%v state=%s err=%v", v, found, state, err)
	}
	if err := cl.NodeCompleted(ctx, rid, epoch, "A", "tool"); err != nil {
		t.Fatal(err)
	}
	if err := cl.RunCompleted(ctx, rid, epoch); err != nil {
		t.Fatal(err)
	}
	// stale lease surfaces as ErrStaleLease
	if err := cl.NodeCompleted(ctx, rid, 999, "B", "tool"); err != worker.ErrStaleLease {
		t.Fatalf("stale lease: want ErrStaleLease, got %v", err)
	}
}

func TestCounterExecutor(t *testing.T) {
	ex := worker.CounterExecutor{}
	if got := ex.Nodes(); len(got) != 2 || got[0] != "A" || got[1] != "B" {
		t.Fatalf("nodes: %v", got)
	}
	st := ex.Run(0, map[string]int{})
	if st["count"] != 1 {
		t.Errorf("A: want count=1, got %d", st["count"])
	}
	st = ex.Run(1, st)
	if st["count"] != 2 {
		t.Errorf("B: want count=2, got %d", st["count"])
	}
}
```

(`newPool`, `seedThreadAssistantRun` live in `execution_integration_test.go`, Task 8; if Task 7 is executed first, add a minimal `worker_testmain_test.go` with the testcontainer TestMain + these helpers, and Task 8 reuses them.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./controlplane/worker/ -run 'TestClientLifecycleAndCheckpoints|TestCounterExecutor' -v`
Expected: FAIL — `undefined: worker.NewClient` / `worker.CounterExecutor`.

- [ ] **Step 3: Write the executor**

Create `controlplane/worker/executor.go`:

```go
// The 2-step counter graph — the thin executor that proves durable execution.
// Node A sets count=1, node B sets count=2. A checkpoint is written after each
// node so a crash between them resumes at B (not A). Deliberately trivial; the
// real graph engine is a later cycle.
package worker

type CounterExecutor struct{}

// Nodes returns the ordered node ids.
func (CounterExecutor) Nodes() []string { return []string{"A", "B"} }

// Run applies node[step] to state and returns the new state. Node A → count=1,
// node B → count=2 (idempotent given step).
func (CounterExecutor) Run(step int, state map[string]int) map[string]int {
	out := map[string]int{}
	for k, v := range state {
		out[k] = v
	}
	out["count"] = step + 1
	return out
}
```

- [ ] **Step 4: Write the HTTP client**

Create `controlplane/worker/client.go` implementing every method in the Interfaces block. Key requirements (write the full file):
- `ErrStaleLease = errors.New("worker: stale lease (409)")`; any epoch-fenced call that gets HTTP 409 returns it.
- `RunStarted` POSTs `{"events":[{"type":"run.started"}]}` to `/api/v1/workers/{wid}/runs/{rid}/events` and decodes `RunStartedResponse.lease_epoch`.
- `NodeCompleted`/`RunCompleted`/`RunFailed` POST the corresponding event with `lease_epoch`.
- `WriteCheckpoint` POSTs `CheckpointWriteRequest` to `/api/v1/threads/{tid}/checkpoints`.
- `LatestCheckpoint` GETs `/api/v1/threads/{tid}/checkpoints/latest?run_id={rid}`; 404 → `found=false, err=nil`.
- Non-2xx (other than the mapped 409/404) → a wrapped error including status + body.

Use the protocol types from `controlplane/endpoints` by importing that package (the types are exported), OR duplicate the small structs in the worker package to avoid the import cycle risk. **Preferred: define a tiny local `protocol.go` in the worker package with the same JSON shapes** (worker must not import the endpoints package's server deps). Add `controlplane/worker/protocol.go` with `workerEvent`, `eventsRequest`, `runStartedResponse`, `checkpointWriteRequest`, `checkpointResponse` mirroring the JSON tags in Task 2.

- [ ] **Step 5: Build, run the tests**

Run: `go build ./controlplane/...`
Run: `go test ./controlplane/worker/ -run 'TestClientLifecycleAndCheckpoints|TestCounterExecutor' -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add controlplane/worker/client.go controlplane/worker/executor.go \
  controlplane/worker/protocol.go controlplane/worker/client_test.go \
  controlplane/worker/worker_testmain_test.go
git commit -m "feat(worker): HTTP client (epoch-aware) + 2-step counter executor"
```

---

## Task 8: Worker runner — NATS consume, ack discipline, durable resume (the reliability proof)

**Files:**
- Create: `controlplane/worker/runner.go`
- Test: `controlplane/worker/execution_integration_test.go`

**Interfaces:**
- Consumes: `Client` + `CounterExecutor` (Task 7); `jetstream.JetStream`; the `graph-executor` consumer; the worker endpoints + run-processor.
- Produces: `type Runner` with `NewRunner(js jetstream.JetStream, cl *Client, ex CounterExecutor) *Runner` and `func (r *Runner) Start(ctx context.Context) error` (binds the `graph-executor` consumer, processes `worker.graph.execute` with the ack discipline, blocks until ctx cancels). `processOne(ctx, cmd)` is unit-visible for the resume test, returning `(acked bool, err error)`.

- [ ] **Step 1: Write the failing tests (end-to-end + durable resume)**

Create `controlplane/worker/execution_integration_test.go`. It needs: testcontainer Postgres + tenant migrations, embedded NATS (JetStream), an httptest server mounting the worker endpoints, the run-processor running, and a Runner. Provide `TestMain` (or reuse `worker_testmain_test.go`) that stands all of this up and exposes `testPool`, `natsURL`, `serverURL`.

Two tests:

```go
// TestExecuteRunEndToEnd: create a queued run + publish run.created → run-processor
// dispatches → Runner consumes → counter graph runs → run completed, 2 checkpoints,
// 2 execution_history rows.
func TestExecuteRunEndToEnd(t *testing.T) { /* ... */ }

// TestDurableResume: run node A + checkpoint 1, then simulate worker death BEFORE
// node B (processOne returns without acking, exits early). Re-deliver the command
// to a fresh Runner. Assert: node A executed exactly ONCE across both attempts
// (execution_history has A once), run completes, checkpoints reach version 2.
func TestDurableResume(t *testing.T) { /* ... */ }
```

Write both bodies fully. For `TestDurableResume`, drive it deterministically without relying on the 5-minute ack_wait: expose a Runner hook/option `stopAfterNode int` (default -1) so the first Runner executes node A, writes checkpoint 1, then returns from `processOne` WITHOUT acking (simulating death); then construct a second Runner (no stop), have it `processOne` the same command (fetch a redelivery, or re-invoke `processOne` with the same command payload), and assert:
- `SELECT count(*) FROM execution_history WHERE run_id=$rid AND node_id='A'` == 1 (A ran once — resume skipped it),
- final run status `completed`,
- `SELECT max(version) FROM snapshots WHERE aggregate_id=$rid` == 2,
- the dead worker's `lease_epoch` writes (if any replayed) return `ErrStaleLease`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./controlplane/worker/ -run 'TestExecuteRunEndToEnd|TestDurableResume' -v`
Expected: FAIL — `undefined: worker.NewRunner`.

- [ ] **Step 3: Write the runner**

Create `controlplane/worker/runner.go`. Full implementation with this control flow in `processOne(ctx, cmd)`:

```
parse cmd → runID, threadID
epoch, err := client.RunStarted(runID)
  if ErrStaleLease → run already terminal/owned → return acked=true (ack, nothing to do)
  if transient err → return acked=false (Nak/redeliver)
resumeFrom := 0
if v, _, found, _ := client.LatestCheckpoint(threadID, runID); found { resumeFrom = v } // v = last completed node index (1-based)
state := loaded-or-empty
for step := resumeFrom; step < len(nodes); step++ {
    if r.stopAfterNode >= 0 && step > r.stopAfterNode { return acked=false } // simulate death (no ack)
    state = executor.Run(step, state)
    if err := client.WriteCheckpoint(threadID, runID, epoch, step+1, mustJSON(state)); err != nil {
        if ErrStaleLease { return acked=true }  // superseded → stop, ack
        return acked=false                        // transient → redeliver
    }
    if err := client.NodeCompleted(runID, epoch, nodes[step], "tool"); err != nil {
        if ErrStaleLease { return acked=true }
        return acked=false
    }
}
if err := client.RunCompleted(runID, epoch); err != nil {
    if ErrStaleLease { return acked=true }
    return acked=false
}
return acked=true
```

- `Start` binds the `graph-executor` durable consumer (`js.Consumer(ctx, "WORKER_COMMANDS", "graph-executor")`), and for each message: `msg.InProgress()` before work, then `acked, err := processOne(...)`; `if acked { msg.Ack() } else { msg.Nak() }`. A deterministic graph error (not implemented for the counter, but wire the branch) → `RunFailed` then `acked=true`.
- `resumeFrom` uses the checkpoint version as the count of completed nodes: version `v` means nodes `0..v-1` are done, so resume at step `v`. This is what makes A run exactly once on redelivery.

- [ ] **Step 4: Build, run the tests**

Run: `go build ./... && go vet ./controlplane/...`
Run: `go test ./controlplane/worker/ -run 'TestExecuteRunEndToEnd|TestDurableResume' -v`
Expected: PASS — including the durable-resume proof (node A once, run completed, checkpoint v2).

- [ ] **Step 5: Full regression**

Run: `go test ./controlplane/...`
Expected: PASS across endpoints, nats, worker, server.

- [ ] **Step 6: Commit**

```bash
git add controlplane/worker/runner.go controlplane/worker/execution_integration_test.go
git commit -m "feat(worker): runner — NATS consume, ack discipline, durable checkpoint resume"
```

---

## Task 9: cmd/worker binary + server wiring for run-processor

**Files:**
- Create: `cmd/worker/main.go`
- Modify: `controlplane/server/server.go` (start the run-processor when NATS is enabled)
- Test: `controlplane/server/server_integration_test.go` (assert run-processor wired) + a build check for cmd/worker

**Interfaces:**
- Consumes: `worker.NewRunner`, `worker.NewClient`, `worker.CounterExecutor`, `nats.Connect`, `nats.NewRunProcessor`, `server.Config`.
- Produces: a runnable `cmd/worker` binary reading env (`DURAGRAPH_API_URL`, `NATS_URL`, `WORKER_ID` optional → random, `WORKER_GRAPHS=counter`); the server starts the `run-processor` alongside the relay when `cfg.NATSURL != "" && cfg.Relays`.

- [ ] **Step 1: Write the failing test**

Add to `controlplane/server/server_integration_test.go` a test asserting that when the server is constructed with NATS enabled, a run.created flows to a worker.graph.execute command (i.e., the run-processor goroutine is actually started by the server). If the existing server test already stands up NATS, extend it; otherwise add `TestServerStartsRunProcessor` that: builds the server with `NATSURL` set + `Relays: true`, publishes a `run.created`, and asserts a `worker.graph.execute` appears on `WORKER_COMMANDS` within 5s.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./controlplane/server/ -run TestServerStartsRunProcessor -v`
Expected: FAIL — server does not start a run-processor yet.

- [ ] **Step 3: Wire the run-processor into the server**

In `controlplane/server/server.go`, where the relay + cleanup worker are constructed (inside the `cfg.NATSURL != ""` block), also construct `nats.NewRunProcessor(js, publisher)` and store it on `Server`. In `Run`, when `s.cfg.Relays`, `go func() { s.rpDone <- s.runProcessor.Start(ctx) }()`. In `Shutdown`, call `s.runProcessor.Stop()` and drain `s.rpDone` like the relay. Add the `runProcessor *nats.RunProcessor` and `rpDone chan error` fields.

- [ ] **Step 4: Write cmd/worker**

Create `cmd/worker/main.go`: read env, connect NATS (`nats.Connect`), ensure streams/consumers are present (or assume the server ensured them — call `nats.EnsureConsumers` defensively), build a `worker.Client` (API URL + random/env worker id), `worker.NewRunner(js, client, worker.CounterExecutor{})`, register, start a heartbeat goroutine (ticker ~20s calling `client.Heartbeat`), `runner.Start(ctx)`, and on SIGINT/SIGTERM `client.Deregister` + cancel. Mirror the graceful-shutdown shape in `controlplane/server/server.go`.

- [ ] **Step 5: Build, run the tests**

Run: `go build ./... ` (compiles `cmd/worker`)
Run: `go test ./controlplane/server/ -run TestServerStartsRunProcessor -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/worker/main.go controlplane/server/server.go controlplane/server/server_integration_test.go
git commit -m "feat(worker): cmd/worker binary + server wires run-processor consumer"
```

---

## Task 10: Spec reconcile — endpoint-queries.d2 workers block

**Files:**
- Modify: `spec/models/d2/endpoint-queries.d2` (spec repo `/home/qwe/platform/duragraph-org/spec`, branch `spec/models-diagrams`)

**Interfaces:** none (spec).

- [ ] **Step 1: Update the workers block to the push model**

In `spec/models/d2/endpoint-queries.d2`, in the `workers_ep` block:
- Add a note that dispatch is push: `# Dispatch: run.created → run-processor consumer → worker.graph.execute (WORKER_COMMANDS). claim is DEFERRED (pull path).`
- On the `claim` entry, prepend `[DEFERRED — pull path replaced by push dispatch]` to its label.
- Rewrite `stream_events` to note it carries run lifecycle too: change its class rows to reflect `run.started` (leases: `status='in_progress', worker_id, lease_epoch+1`; no `runs.lease_expires_at`), `execution.node_*`, and `run.completed`/`run.failed`, all `lease_epoch`-fenced (409 on mismatch).
- On `write_checkpoint`, note the `(stream_id, version)` upsert + `lease_epoch` fence, and add a `latest_checkpoint` note for the resume lookup (`SELECT ... WHERE aggregate_id=run_id ORDER BY version DESC LIMIT 1`).

- [ ] **Step 2: Optional d2 render check**

Run: `command -v d2 >/dev/null && d2 /home/qwe/platform/duragraph-org/spec/models/d2/endpoint-queries.d2 /tmp/eq.svg && echo "d2 OK" || echo "d2 not installed — skipping"`

- [ ] **Step 3: Commit in the spec repo**

```bash
git -C /home/qwe/platform/duragraph-org/spec add models/d2/endpoint-queries.d2
git -C /home/qwe/platform/duragraph-org/spec commit -m "spec(models): worker path is push (run-processor → worker.graph.execute); events endpoint carries run lifecycle; claim deferred"
```

---

## Self-Review

**Spec coverage** (design doc → task):
- Migration 006 snapshots unique → Task 1. ✓
- Worker protocol types + rows → Task 2. ✓
- register/heartbeat/deregister → Task 3. ✓
- events (run lifecycle + node, epoch-fenced) → Task 4. ✓
- checkpoints (epoch-fenced upsert + read + latest-for-run) → Task 5. ✓
- run-processor dispatch + max_deliver → Task 6. ✓
- worker client + counter executor → Task 7. ✓
- runner ack discipline + durable resume proof → Task 8. ✓
- cmd/worker + server wiring → Task 9. ✓
- spec reconcile → Task 10. ✓

**Placeholder scan:** the two `var _ =` lines in Task 3 are explicitly introduced and explicitly removed in Task 4 Step 3 (not a dangling stub). Client method bodies in Task 7 Step 4 are specified by contract + JSON shapes rather than transcribed line-for-line — the interface list gives every signature, endpoint, and status mapping; this is the one place the plan describes rather than transcribes, and it is bounded (a thin HTTP client). Runner control flow (Task 8 Step 3) is given as explicit pseudocode with every branch named.

**Type consistency:** `WorkerEvent.LeaseEpoch`, `RunStartedResponse.LeaseEpoch`, `CheckpointWriteRequest.{RunID,LeaseEpoch,Version,State}` are used identically across Tasks 2/4/5/7. Epoch fencing is atomic (in-SQL `WHERE ... lease_epoch=$epoch`, 0 rows → 409) in Tasks 4 and 5 — no separate read; the server-side `errStaleLease` sentinel (Task 4) is the shared mechanism. `snapshotRow` defined in Task 2, used in Task 5. The worker-side `ErrStaleLease` (Task 7, a distinct client-side sentinel for a 409 response) is consumed by the runner in Task 8. Handler names match `pascal(workers)+pascal(endpoint name)` from the generator (`WorkersStreamEvents`, `WorkersWriteCheckpoint`, `WorkersLatestCheckpoint`).

**Open risks flagged for the implementer:**
- `SubjectFor("worker.graph.execute")` must map to the `worker_commands` category — Task 6 Step 4 says verify `categoryFor` and extend if needed. Do not assume.
- Echo route precedence for `/threads/{tid}/checkpoints/latest` vs `/threads/{tid}/checkpoints/{ckpt}` — Task 5 Step 3 notes it; if Echo mis-routes, the `latest` handler still guards on `run_id`.
- Task 7 test harness (`newPool`, `TestMain`) is shared with Task 8; whichever runs first creates `worker_testmain_test.go`.

---

## Notes for the implementer

- `runs` has NO `lease_expires_at`. If any SQL you write references it on `runs`, that is a bug — fence on `lease_epoch` only.
- `execution_history.node_type` ∈ {start,end,llm,tool,conditional}; the counter nodes use `tool`. `status` ∈ {started,completed,failed,skipped}.
- Do not import the `endpoints` package from the `worker` package for anything but is avoided entirely — the worker talks HTTP; protocol structs are duplicated in `controlplane/worker/protocol.go`.
- Do not push/PR/merge without explicit approval.
