# Tenant CRUD Completion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fill 8 remaining tenant read/write stubs (assistants graph read; thread state/history/checkpoint reads + write; run cancel-stateless/batch/thread-copy) as bespoke hand-written handlers.

**Architecture:** All 8 are marked `custom: true` (routes generated, bodies hand-written), mapping DB rows to the existing OpenAPI types (`GraphSchema`, `ThreadState`, `Run`, `Thread`) — the manifest needs no `response:` binding because custom handlers hand-map. Reuse `snapshotRow`/`runRow`/`threadRow`; add `graphRow`.

**Tech Stack:** Go 1.25, Echo v4, pgx v5, testcontainers Postgres.

## Global Constraints

- Design doc: `controlplane/docs/superpowers/specs/2026-08-06-tenant-crud-completion-design.md`.
- All 8 endpoints are `custom` hand-written handlers; verify the exact fields of the OpenAPI response types (`GraphSchema`, `ThreadState`, `Run`, `Thread`) in `types_gen.go` and hand-map what corresponds, recording any divergence in `rows.go`'s DIVERGENCES block (the established pattern).
- Thread-scoping is required on thread reads: a checkpoint/state belonging to another thread's runs → 404, never leaked.
- Writes emit their domain event via `writeTx` + the projection (mirror the existing create/cancel impls): `run.cancelled` (cancel_stateless), `run.created`×N (batch), `thread.created` (copy). `create_checkpoint` is a plain `snapshots` INSERT (infrastructure, no event — mirror the worker's `write_checkpoint`, but client-driven so NO lease_epoch fence).
- Generated `*_gen.go` never hand-edited; the `custom` flag change must leave other groups byte-identical after regen.
- Integration tests: testcontainers Postgres, no build tag, standard CI. Never weaken assertions.
- Never hand-edit go.mod/go.sum. Conventional commits. No PR/push/merge without explicit approval (the controller handles PR/merge).
- Run from the worktree root `~/worktrees/duragraph/feat/controlplane-server` (branch `feat/controlplane-tenant-crud`).

## File Structure

- `controlplane/gen/endpoints.yaml` — `custom: true` on the 8 endpoints.
- `controlplane/endpoints/{assistants,threads,runs}_gen.go` (regenerate) — routes-only for the 8.
- `controlplane/endpoints/rows.go` — add `graphRow` + `toAPI() GraphSchema`; a `snapshotRow.toThreadState()` mapper.
- `controlplane/endpoints/threads_state.go` (new) — `ThreadsGetState`, `ThreadsGetCheckpointState`, `ThreadsGetHistory`, `ThreadsCreateCheckpoint`, `ThreadsCopy`.
- `controlplane/endpoints/assistants_graph.go` (new) — `AssistantsGetGraph`.
- `controlplane/endpoints/runs_writes.go` (new) — `RunsCancelStateless`, `RunsBatchCreate`.
- Tests alongside.

---

## Task 1: Reads — assistant graph + thread state/checkpoint/history

**Files:**
- Modify: `controlplane/gen/endpoints.yaml` (custom on `get_graph`, `get_state`, `get_checkpoint_state`, `get_history`)
- Regenerate: `assistants_gen.go`, `threads_gen.go`
- Modify: `controlplane/endpoints/rows.go` (`graphRow`+`toAPI`, `snapshotRow.toThreadState`)
- Create: `controlplane/endpoints/assistants_graph.go`, `controlplane/endpoints/threads_state.go`
- Test: `controlplane/endpoints/tenant_reads_integration_test.go`

**Interfaces:**
- Produces: `graphRow`+`toAPI() GraphSchema`; `snapshotRow.toThreadState() ThreadState`; handlers `AssistantsGetGraph`, `ThreadsGetState`, `ThreadsGetCheckpointState`, `ThreadsGetHistory`.
- Consumes: `GraphSchema`, `ThreadState` (types_gen.go — read their fields), `snapshotRow` (rows.go).

- [ ] **Step 1: Write the failing tests**

Create `controlplane/endpoints/tenant_reads_integration_test.go`:
- `TestAssistantGetGraph`: seed an assistant + a `graphs` row (nodes/edges/config); `GET /api/v1/assistants/{id}/graph` → 200, assert graph fields (nodes/edges); missing assistant → 404.
- `TestThreadGetState`: seed a thread + a run on it + 2 `snapshots` (versions 1,2); `GET /api/v1/threads/{tid}/state` → 200 returns the LATEST (version 2) state; a thread with no snapshots → 404 (or empty per the ThreadState contract — pick and assert one).
- `TestThreadGetCheckpointState`: `GET /threads/{tid}/state/{ckpt}` → 200 for the thread's snapshot; a snapshot id from ANOTHER thread's run → 404 (thread-scoping).
- `TestThreadGetHistory`: `GET /threads/{tid}/history` → list of the thread's snapshots newest-first.

(Reuse the shared `testPool`; seed helpers may mirror the store/worker tests.)

- [ ] **Step 2: Run — fails (stubs)**

Run: `go test ./controlplane/endpoints/ -run 'TestAssistantGetGraph|TestThreadGet' -v`
Expected: FAIL (sse/read stubs).

- [ ] **Step 3: Mark custom + regen**

`custom: true` on `get_graph`, `get_state`, `get_checkpoint_state`, `get_history` in `endpoints.yaml`; `go run ./controlplane/gen`; confirm other groups byte-identical (`git diff --stat` empty for them); `assistants_gen.go`/`threads_gen.go` route `AssistantsGetGraph`/`ThreadsGetState`/`ThreadsGetCheckpointState`/`ThreadsGetHistory` bodiless.

- [ ] **Step 4: Add rows + mappers**

In `rows.go`: add `graphRow` (db tags for `graphs`: id, assistant_id, name, version, description, nodes []byte, edges []byte, config []byte, timestamps) + `toAPI() GraphSchema` (read `GraphSchema`'s fields in `types_gen.go`; map nodes/edges/config, best-effort unmarshal jsonb; record divergences). Add `func (r snapshotRow) toThreadState() ThreadState` (map `state`→values and whatever `ThreadState` defines; leave unmapped fields zero, record divergence).

- [ ] **Step 5: Write the read handlers**

`assistants_graph.go`: `AssistantsGetGraph` — `SELECT ... FROM graphs WHERE assistant_id=$1` (or by the assistant's graph_id) → `graphRow` → `c.JSON(200, row.toAPI())`; `pgx.ErrNoRows` → 404.

`threads_state.go`: `ThreadsGetState` — latest snapshot across the thread's runs: `SELECT ... FROM snapshots WHERE aggregate_id IN (SELECT id FROM runs WHERE thread_id=$1) ORDER BY version DESC LIMIT 1` → `toThreadState()`; none → 404. `ThreadsGetCheckpointState` — `SELECT ... FROM snapshots WHERE id=$ckpt AND aggregate_id IN (SELECT id FROM runs WHERE thread_id=$tid)` (thread-scoped) → 404 if absent. `ThreadsGetHistory` — `SELECT ... FROM snapshots WHERE aggregate_id IN (thread's runs) ORDER BY version DESC` → `[]ThreadState`.

- [ ] **Step 6: Build + tests + regression**

Run: `go build ./controlplane/... && go test ./controlplane/endpoints/ -run 'TestAssistantGetGraph|TestThreadGet' -v -count=1` → PASS.
Run: `go test ./controlplane/endpoints/ -count=1` → PASS (other groups green).

- [ ] **Step 7: Commit**

```bash
git add controlplane/gen/endpoints.yaml controlplane/endpoints/assistants_gen.go \
  controlplane/endpoints/threads_gen.go controlplane/endpoints/rows.go \
  controlplane/endpoints/assistants_graph.go controlplane/endpoints/threads_state.go \
  controlplane/endpoints/tenant_reads_integration_test.go
git commit -m "feat(controlplane): assistant graph read + thread state/checkpoint/history reads"
```

---

## Task 2: Writes — create_checkpoint, cancel_stateless, batch_create, copy

**Files:**
- Modify: `controlplane/gen/endpoints.yaml` (custom on `create_checkpoint`, `cancel_stateless`, `batch_create`, `copy`)
- Regenerate: `threads_gen.go`, `runs_gen.go`
- Modify: `controlplane/endpoints/threads_state.go` (add `ThreadsCreateCheckpoint`, `ThreadsCopy`)
- Create: `controlplane/endpoints/runs_writes.go` (`RunsCancelStateless`, `RunsBatchCreate`)
- Test: `controlplane/endpoints/tenant_writes_integration_test.go`

**Interfaces:**
- Consumes: `writeTx`, `Event`, `mustJSON`, `runRow`/`threadRow`, the existing `RunsCancel`/`RunsCreateStateless` SQL as the mirror.
- Produces: `ThreadsCreateCheckpoint`, `ThreadsCopy`, `RunsCancelStateless`, `RunsBatchCreate`.

- [ ] **Step 1: Write the failing tests**

Create `controlplane/endpoints/tenant_writes_integration_test.go`:
- `TestCancelStateless`: seed a queued/in_progress stateless run; `POST /api/v1/runs/cancel` (body with run_id) → run status `cancelled`, `run.cancelled` in outbox.
- `TestBatchCreate`: `POST /api/v1/runs/batch` with N create specs → N `runs` rows + N `run.created` outbox rows in ONE tx; returns the ids.
- `TestCreateCheckpoint`: seed a thread + run (+ its event_stream via a run.started, so snapshots FK resolves); `POST /threads/{tid}/state/checkpoint` → a `snapshots` row written; read it back via `get_checkpoint_state` (from Task 1) round-trips.
- `TestThreadCopy`: seed a source thread with a snapshot (state); `POST /threads/{tid}/copy` → a NEW thread row + `thread.created` outbox row; the new thread carries the copied state (assert via get_state).

- [ ] **Step 2: Run — fails**

Run: `go test ./controlplane/endpoints/ -run 'TestCancelStateless|TestBatchCreate|TestCreateCheckpoint|TestThreadCopy' -v` → FAIL.

- [ ] **Step 3: Mark custom + regen**

`custom: true` on `create_checkpoint`, `cancel_stateless`, `batch_create`, `copy`; regen; confirm other groups byte-identical; the 4 handlers route bodiless.

- [ ] **Step 4: Write the handlers**

`runs_writes.go`:
- `RunsCancelStateless` — mirror `RunsCancel` (which is impl `update` mode: `writeTx([run.cancelled]) + UPDATE runs SET status='cancelled' WHERE id=$rid`), keyed on the body's run id (stateless — no thread param). Read the generated `RunsCancel` in `runs_gen.go` for the exact SQL and mirror it.
- `RunsBatchCreate` — bind a list of create specs; in ONE `writeTx`-style transaction append `run.created` per spec + `INSERT runs` per spec, single `pg_notify` at end; return the created run ids. (writeTx does one pg_notify already; for a batch, either call a batch-aware path or open the tx directly appending N events + N inserts + one notify — mirror `endpoint-queries.d2` batch_create.)

`threads_state.go` (append):
- `ThreadsCreateCheckpoint` — resolve the run/thread's `stream_id` from `event_streams` (like the worker's `WorkersWriteCheckpoint`), then INSERT `snapshots` (no lease fence — client checkpoint). Return the checkpoint id. (Reuse the worker checkpoint SQL shape; it is client-driven here.)
- `ThreadsCopy` — read the source thread's latest snapshot state; `writeTx([thread.created]) + INSERT threads` seeded with that state (values); return the new `Thread`. Thread-scoped read of the source.

- [ ] **Step 5: Build + tests + full regression**

Run: `go build ./... && go test ./controlplane/endpoints/ -run 'TestCancelStateless|TestBatchCreate|TestCreateCheckpoint|TestThreadCopy' -v -count=1` → PASS.
Run: `go test ./controlplane/... -count=1` → whole rebuild green.

- [ ] **Step 6: Commit**

```bash
git add controlplane/gen/endpoints.yaml controlplane/endpoints/threads_gen.go \
  controlplane/endpoints/runs_gen.go controlplane/endpoints/threads_state.go \
  controlplane/endpoints/runs_writes.go controlplane/endpoints/tenant_writes_integration_test.go
git commit -m "feat(controlplane): run cancel-stateless + batch-create + thread checkpoint + copy"
```

---

## Self-Review

**Spec coverage:** get_graph → T1; get_state/get_checkpoint_state/get_history → T1; create_checkpoint/cancel_stateless/batch_create/copy → T2. Deferred (resume, get_versions, set_latest) explicitly out of scope. ✓

**Placeholder scan:** the row→type mappers (`graphRow.toAPI`, `snapshotRow.toThreadState`) are specified by contract ("read `GraphSchema`/`ThreadState` fields in types_gen.go and map what corresponds, record divergence") rather than transcribed, because the exact target fields must be read from the generated types — this matches how every other `toAPI` in `rows.go` was built and is the correct just-in-time step, not a vague requirement. All SQL and handler control flow are concrete.

**Type consistency:** handler names match `pascal(group)+pascal(name)`: `AssistantsGetGraph`, `ThreadsGetState`, `ThreadsGetCheckpointState`, `ThreadsGetHistory`, `ThreadsCreateCheckpoint`, `ThreadsCopy`, `RunsCancelStateless`, `RunsBatchCreate`. `snapshotRow` reused across get_checkpoint_state/get_state/get_history/create_checkpoint/copy.

**Open risks:**
- Verify `GraphSchema`/`ThreadState` field names against `types_gen.go` before mapping (a wrong field name fails the build; a missing concept → record a divergence, don't invent).
- `create_checkpoint` needs the run/thread's `event_stream` to exist (snapshots FK). For a thread with no run yet, decide: 404 / create-a-stream — match the worker's `write_checkpoint` behavior (it requires the stream from run.started).
- `batch_create` must do ONE `pg_notify` for the whole batch (not per run) per `endpoint-queries.d2`.

## Notes for the implementer

- All 8 handlers are `custom` — hand-map rows to the OpenAPI types; the manifest needs no `response:` binding.
- Thread reads MUST be thread-scoped (a resource from another thread → 404).
- Do not push/PR/merge without explicit approval (the controller handles that).
