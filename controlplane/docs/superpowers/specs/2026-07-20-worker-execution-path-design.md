# Worker execution path — thin vertical slice (with durable checkpoints)

**Date:** 2026-07-20
**Branch:** `feat/controlplane-worker-execution`
**Status:** approved design, pending implementation plan

## Context

The control-plane rebuild (`controlplane/`) builds bottom-up from the **structural**
d2 diagrams in `spec/models/d2/`. Layers 1–4 and endpoint groups
assistants/threads/runs/crons/store/system are merged. This design covers the
**worker execution path**: the mechanism that turns a queued run into an executed
one.

### Source-of-truth discipline (important)

The rebuild builds ONLY from the **structural** d2 set:
`workers.d2`, `nats.d2`, `endpoint-queries.d2` (workers block), `postgres.d2`
(migrations `001` snapshots + `005` workers). The **`code-*.d2` files are
excluded** — their headers declare `internal/application/*`, i.e. they document
the *old* `internal/` DDD architecture (`WorkerService`, `TaskRepository`,
`ClaimTask`/`CompleteTask`, DDD aggregates) that the rebuild replaces. Reaching
for that model is how the rebuild drifts back into the bloat it exists to remove.
Where the structural d2 leaves a gap, we fill it with a spec-first decision, not
by importing the stale model.

### Scope of this cycle: A + B + C-thin, **with checkpoints**

- **A. Worker HTTP endpoints** — register, heartbeat, deregister, events,
  write_checkpoint, read_checkpoint (6 of the 7 workers-block endpoints).
- **B. Dispatch** — the `run-processor` NATS consumer.
- **C. Worker binary (thin)** — `cmd/worker` running a **2-step counter graph**
  with durable checkpoint-based resume.

**Deferred (out of scope), recorded so nothing is silently dropped:**
- `claim` endpoint — the pull path; replaced by push dispatch (below).
- Runs SSE / `wait` / `stream` endpoints — pass D.
- A real graph engine (beyond the 2-step counter), interrupts/HITL, the SDK
  extraction, multi-graph routing (`llm`/`tool` worker command families).

### Dispatch model: push, not poll

Decision: **push via NATS commands**, not worker polling. Polling cannot tell you
where a worker got disconnected; a JetStream durable consumer with explicit ack
can — an un-acked message *is* the durable record of in-flight work, and
`ack_wait` expiry redelivers it to a healthy worker. This is the reliability
spine. It aligns with `nats.d2` (`WORKER_COMMANDS` stream, `worker.graph.execute`,
`graph-executor` consumer with `ack_wait=5m`, `AckExplicit`) and against the pull
`claim` path in `endpoint-queries.d2` (which is why `claim` is deferred).

### Reliability rests on two legs — ack AND checkpoints

1. **Ack/redelivery** answers *"run it again, somewhere healthy"* when a worker
   dies.
2. **Checkpoints** answer *"continue where it died"* instead of restarting from
   zero. Without them, redelivery re-runs every LLM call and re-fires every tool
   side effect — the exact unreliability push mode exists to avoid. The d2 makes
   dispatch checkpoint-aware: `claim` returns `checkpoint_id` *"(null if fresh,
   set if resuming)"* and `write_checkpoint`/`read_checkpoint` persist/read the
   `snapshots` table.

A single-node echo graph cannot exercise resume (no intermediate state), so the
thin graph is **2 steps** (node A → checkpoint → node B). That is the smallest
graph that proves durable execution.

## Architecture

### Flow

```
client → POST /runs → run.created (event + outbox) → RUNS stream
run-processor consumer (RUNS, filter run.created)
    → publish worker.graph.execute → WORKER_COMMANDS   [Nats-Msg-Id = run_id]
worker: durable pull consumer (graph-executor, ack_wait=5m, AckExplicit)
  on delivery:
    read latest checkpoint for run_id (snapshots, version DESC)
    POST events run.started        → re-lease run (in_progress, worker_id, lease_epoch++); returns epoch
    for each node not yet checkpointed:
        execute node
        POST /threads/{tid}/checkpoints   (snapshot state after node; epoch-fenced)
        POST events node_completed         (carries lease_epoch; 409 if stale)
    POST events run.completed        → 200 → msg.Ack()
  failure handling:
    transient (DB/HTTP 5xx, crash, panic) → Nak / no-ack → ack_wait redelivers
    deterministic graph error            → POST run.failed → Ack (no redelivery)
    max_deliver exceeded                 → dead-letter → run.failed
  msg.InProgress() heartbeats during execution (extend ack deadline past ack_wait)
```

### Components

**1. `run-processor` consumer** (`controlplane/nats` + wired in `controlplane/server`)
- Durable pull consumer on `RUNS`, filter `duragraph.runs.run.created`.
- On a queued run: publish `worker.graph.execute` to `WORKER_COMMANDS` with
  `Nats-Msg-Id = run_id` (JetStream dedup) and a payload of
  `{run_id, thread_id, assistant_id, graph_id, input}`.
- Thin dispatcher only — it does NOT mutate run state (the worker leases via
  `run.started`). Acks after successful publish.

**2. Worker HTTP endpoints** (`controlplane/endpoints/workers.go`, generator
`custom` flag; hand-defined request/response types from the d2)
- `POST /workers/register` → `INSERT workers(worker_id, graphs, capacity,
  status='online', lease_expires_at)`, upsert `graphs`. → 200 `WorkerRegistered`.
- `POST /workers/{id}/heartbeat` → `UPDATE workers SET status, active_runs,
  lease_expires_at=now()+60s WHERE worker_id=$id AND lease_expires_at > now()`.
  → 200 `{commands: []}` (drain/shutdown/update_config when present; empty for
  the slice).
- `POST /workers/{id}/deregister` → `UPDATE workers SET status='offline'` +
  requeue its in-flight runs (`UPDATE runs SET status='queued', worker_id=NULL
  WHERE worker_id=$id AND status='in_progress'`). → 204.
- `POST /workers/{id}/runs/{rid}/events` — the single worker→server state
  channel. **`run.started` is a discrete call** (it establishes the lease and
  returns the epoch); `node_*` and terminal events are separate calls (batchable
  among themselves) that **carry** that epoch. Each event applies its projection +
  outbox via `writeTx`:
  - `run.started`: re-lease the run — `UPDATE runs SET status='in_progress',
    worker_id=$id, lease_epoch=lease_epoch+1, started_at=COALESCE(started_at,now())
    WHERE id=$rid AND status IN ('queued','in_progress')`; emit `run.started`.
    Returns the new `lease_epoch`. If the run is already terminal
    (`completed`/`failed`) no row updates → **409** (worker acks; nothing to do).
    **There is no `runs.lease_expires_at`** — redelivery itself is the "prior
    worker presumed dead" signal, and the epoch bump fences the prior worker out.
  - `node_started` / `node_completed`: `INSERT execution_history(...)` + emit
    `execution.node_*`; reject if `body.lease_epoch != runs.lease_epoch` → 409.
  - `run.completed` / `run.failed`: `UPDATE runs SET status='completed'|'failed',
    completed_at=now() WHERE id=$rid AND lease_epoch=$epoch`; emit the event;
    epoch-guarded (0 rows → 409).
- `POST /threads/{tid}/checkpoints` (write_checkpoint) — **epoch-fenced** like the
  events endpoint (`body.lease_epoch` must match `runs.lease_epoch`, else 409, so
  a dead worker cannot poison resume state). Resolves `stream_id` from the run's
  stream (`SELECT stream_id FROM event_streams WHERE aggregate_type='Run' AND
  aggregate_id=$rid` — guaranteed to exist because `run.started` created it via
  `writeTx`'s EnsureStream; ordering dependency). Then **upsert**
  `INSERT snapshots(stream_id, aggregate_type='Run', aggregate_id=$rid, version,
  state) ON CONFLICT (stream_id, version) DO UPDATE SET state=EXCLUDED.state`
  (requires the new unique constraint, migration `006`). No outbox. `version` =
  index of the node just completed. → 200 `{checkpoint_id}`.
- `GET /threads/{tid}/checkpoints/{ckpt}` (read_checkpoint) → `SELECT * FROM
  snapshots WHERE id=$ckpt AND aggregate_id IN (SELECT id FROM runs WHERE
  thread_id=$tid)`. → 200 `Checkpoint` / 404. The worker's **resume lookup** is a
  distinct latest-for-run query (`SELECT ... WHERE aggregate_id=$rid ORDER BY
  version DESC LIMIT 1`); with the `(stream_id, version)` unique constraint it is
  unambiguous.

**Migration `006_worker_checkpoint.up.sql`** — `ALTER TABLE snapshots ADD
CONSTRAINT uq_snapshots_stream_version UNIQUE (stream_id, version)` (forward-only;
required for checkpoint upsert idempotency under redelivery).

**3. Worker binary** (`cmd/worker/main.go` + `controlplane/worker/` package)
- `controlplane/worker/` holds: the NATS `graph-executor` pull consumer + ack
  loop, the HTTP event/checkpoint client (calls the control-plane endpoints), and
  the **2-step counter executor**.
- **Counter graph:** state `{count:int}`. Node A: `count=1`. Node B: `count=2`.
  A checkpoint is written after each node (`version=1` after A, `version=2` after
  B). `run.completed` output = final state.
- **Resume:** on delivery, read latest checkpoint for `run_id`. If `version>=1`,
  skip node A and resume at node B; if none, start fresh. This is what a redeliver
  after a mid-run crash exercises.
- Registers on startup; heartbeat goroutine (~20s); deregisters on SIGINT/SIGTERM.

### The lease_epoch idempotency mechanic

`runs.lease_epoch` (migration `003`, a fencing token) is the redelivery guard.
Note: there is **no** `lease_expires_at` on `runs` — that column exists only on
`workers`. The run has no timestamp lease; JetStream's `ack_wait` redelivery is
the sole "prior worker is gone" trigger, and `lease_epoch` is the fence.
- Every non-terminal `run.started` increments `lease_epoch` and stamps
  `worker_id`, and returns the new epoch. The worker carries that epoch on every
  subsequent event **and checkpoint** write.
- A resurrected or duplicate worker holding a *stale* epoch is rejected (409) on
  its next write (event or checkpoint) — its lease was superseded.
- A redelivered command re-leases (epoch++), so the new worker owns the run and
  resumes from the latest checkpoint; the dead worker's late writes bounce.
- `run.started` on an already-terminal run → 409 (the worker acks; the run is
  done). This is the redelivery-after-completion case (worker died between the DB
  write and the ack).

## Spec-first change (precedes implementation)

Reconcile `endpoint-queries.d2`'s workers block to this design (in the spec repo):
- Replace the `claim` pull path with the push dispatch note (run.created →
  run-processor → worker.graph.execute), and mark `claim` deferred.
- Generalize `stream_events` to carry run lifecycle (`run.started` /
  `run.completed` / `run.failed`) in addition to `execution.node_*`, with the
  `lease_epoch` guard.
- Keep `write_checkpoint` / `read_checkpoint` as specified; note the
  latest-for-run resume lookup.

The worker↔control-plane protocol is **internal and native** (not the public
LangGraph API), so its Go request/response types are hand-defined **from the d2**
rather than added to `duragraph-latest.yaml`. Adding an internal worker protocol
to the public API contract would be wrong; the d2 is its spec.

## Error handling

- Endpoints: `echo.NewHTTPError(status, msg)`; bind failure → 400; `pgx.ErrNoRows`
  → 404; `lease_epoch` mismatch / already-leased → **409**; unexpected DB error →
  500.
- Worker: transient (HTTP 5xx, connection failure, panic) → `Nak`/no-ack →
  redeliver; deterministic graph error → post `run.failed` then `Ack`;
  `max_deliver` exceeded → the run is marked `run.failed` (dead-letter).

## Testing

Testcontainers Postgres + **embedded in-process NATS** (per repo convention; no
build tag, runs in standard CI):

1. **Endpoint unit/integration** — register / heartbeat (lease-valid + expired) /
   deregister (requeues in-flight) / events happy path / `write_checkpoint` +
   `read_checkpoint` round-trip / **lease-stale event → 409**.
2. **End-to-end execution** — create run → `run-processor` publishes command →
   in-process worker (2-step) consumes → assert final DB state: run `completed`,
   2 rows in `execution_history`, 2 `snapshots` (version 1 + 2), and the
   `run.started`/`node_completed`×2/`run.completed` events in `outbox`.
3. **Durable resume (the reliability proof)** — run node A + write checkpoint 1,
   then simulate worker death (worker acks nothing, exits before node B) → let
   `ack_wait` redeliver (or force redelivery) → a second worker reads checkpoint 1
   and resumes at node B → assert **node A executed exactly once** (via an
   execution-count / side-effect counter), run reaches `completed`, and the dead
   worker's stale-epoch write (if any) is rejected 409.
4. **run-processor dedup** — duplicate `run.created` → single `worker.graph.execute`
   (Nats-Msg-Id).

Green tests are the done-criteria; test 3 is the one that proves the whole point.

## Files (anticipated)

- `controlplane/db/migrations/tenant/006_worker_checkpoint.up.sql` /
  `.down.sql` — `UNIQUE (stream_id, version)` on `snapshots` (checkpoint upsert
  idempotency).
- `controlplane/gen/endpoints.yaml` — mark workers endpoints `custom`; scope to the
  6 in this cycle.
- `controlplane/endpoints/workers.go` — hand-written handlers (custom).
- `controlplane/endpoints/worker_types.go` — hand-defined worker protocol types.
- `controlplane/endpoints/rows.go` — `workerRow`, `snapshotRow`, `execHistoryRow`
  mappers.
- `controlplane/endpoints/workers_gen.go` — regenerated routes-only.
- `controlplane/nats/run_processor.go` — the dispatch consumer.
- `controlplane/worker/` — consumer + ack loop, event/checkpoint HTTP client,
  counter executor.
- `cmd/worker/main.go` — binary entrypoint.
- `controlplane/server/server.go` — wire the run-processor consumer.
- `controlplane/endpoints/workers_integration_test.go`,
  `controlplane/worker/execution_integration_test.go` — tests.
- `spec/models/d2/endpoint-queries.d2` (spec repo) — the reconcile.

## Out of scope (recorded)

`claim` endpoint; runs SSE/`wait`/`stream` (pass D); real graph engine beyond the
counter; `llm`/`tool` worker command families; interrupts/HITL; SDK extraction;
worker `commands` (drain/shutdown/update_config) beyond an empty list;
multi-worker load/fairness.

**Threaded runs only this slice.** `runs.thread_id` is nullable (stateless runs
are valid), but the checkpoint endpoints are thread-scoped
(`POST /threads/{tid}/checkpoints`), so a stateless run cannot checkpoint through
them. The counter-graph test uses a threaded run. Checkpointing stateless runs
(a run-scoped checkpoint path, or synthesizing a thread) is deferred and recorded
here so it is not silently assumed to work.
