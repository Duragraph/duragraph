# Run reaper — fail runs the execution path leaves stuck

**Date:** 2026-07-28
**Branch:** `feat/controlplane-run-reaper` (off `main`)
**Status:** approved design, pending implementation plan

## Context

The worker execution path (merged) recovers a crashed worker's run via JetStream
redelivery + checkpoint resume, and INF-1 escalates a leased-but-failing run to
`run.failed` when `max_deliver` is exhausted. But two residual cases leave a run
non-terminal with nothing to finish it (documented limitation in the INF-1
design):

- **Leased then stranded** — a worker leased the run, then crashed/blipped on its
  final delivery before it could escalate. JetStream has stopped redelivering; the
  run sits `in_progress` forever.
- **Never leased** — the `worker.graph.execute` command was lost or dead-lettered
  before any worker leased the run. It sits `queued` forever (run-processor only
  dispatches on `run.created`, so nothing re-dispatches it).

The reaper is a control-plane safety net that finds these stuck runs and marks
them `failed` (emitting `run.failed` so observers see it). It does **not** recover
or re-dispatch — that is a deliberate follow-up; this is the minimal "no run stays
stuck forever" net.

## Correctness constraint — do not preempt JetStream recovery

The reaper must fail **only** runs the execution path has genuinely given up on.
When a worker dies, JetStream redelivers the un-acked command for up to
`max_deliver × ack_wait` (`5 × 5m ≈ 25 min`), and a fresh worker resumes from the
last checkpoint. Therefore stuck-detection is based on **elapsed time since the run
started/was created**, NOT on a worker's heartbeat lease. A worker-lease trigger
(≈60 s) would fail runs mid-recovery: the run would flip to `failed`, then the
redelivered worker's `run.started` would 409 and give up — defeating durable
resume. The `in_progress` staleness threshold is therefore set **beyond the full
redelivery window**.

## Design

### RunReaper — a background worker

Mirrors `nats.CleanupWorker` (the outbox-cleanup goroutine): a ticker with
`Start(ctx) error` / `Stop()` and a `Done chan error`, wired in `server.go`
alongside relay / cleanup / run-processor (its own `reaperDone` channel, started
in `Run` when enabled, stopped + drained in `Shutdown`).

Config (all tunable, sensible defaults):
- `Interval` — how often it ticks. Default **60 s**.
- `InProgressStaleAfter` — how long an `in_progress` run may sit with no terminal
  transition before it's presumed dead. Default **30 min** (> the ~25 min
  redelivery window, with margin).
- `QueuedStaleAfter` — how long a `queued` run may sit undispatched. Default
  **10 min** (dispatch is normally near-instant; a long-queued run means its
  command was lost).

### Per-tick behavior — one transaction

Each tick opens one tenant-DB transaction and, atomically:

1. Selects stuck runs:
   ```sql
   SELECT id, lease_epoch FROM runs
   WHERE (status = 'in_progress' AND started_at < now() - $inProgressStaleAfter)
      OR (status = 'queued'      AND created_at < now() - $queuedStaleAfter)
   FOR UPDATE SKIP LOCKED
   ```
   (`started_at` is set by `run.started`; a queued run has no `started_at`, so the
   two arms are disjoint. `SKIP LOCKED` avoids contending with a live write.)
2. For each stuck run: `UPDATE runs SET status='failed', completed_at=now(),
   error='reaped: no active worker (stuck <status> past <threshold>)'
   WHERE id=$id`.
3. Emits a `run.failed` domain event for each — through the SAME transactional
   outbox path everything else uses (events + outbox row + `pg_notify`), so the
   relay publishes it to NATS and SSE/observers see the failure. Not a silent DB
   flip.
4. Commits. A failure at any point rolls back the whole tick (no partial reaps).

The reap is intentionally **not** epoch-fenced: the reaper is the authoritative
system actor for abandoned runs, and its threshold guarantees no live worker owns
the run. (If a worker somehow reappears after a reap and posts an event, it hits
the normal terminal-run 409 — the same as any late writer.)

### DRY the event-append (small shared extraction)

`run.failed` must be emitted via the existing reliable append (events + outbox +
notify), which today lives as the endpoints-private `Event` type + `appendEvent`
func (used by `endpoints.writeTx`). To avoid duplicating that append SQL in the
reaper (the exact shape-duplication that caused the run-processor envelope bug),
extract it into a small shared package:

- New `controlplane/eventstore` package: `type Event` (moved) + `func Append(ctx,
  tx pgx.Tx, e Event) error` (the current `appendEvent` body).
- `endpoints`: `writeTx` calls `eventstore.Append`; `endpoints.Event` becomes an
  alias for / re-uses `eventstore.Event` so the many existing handler references
  compile unchanged.
- The reaper opens its tx, updates stuck runs, calls `eventstore.Append(tx,
  runFailedEvent)` per run, fires `pg_notify('outbox_new','')`, commits.

One append implementation, no drift. This extraction is the only change to
already-merged endpoint code, and it is behavior-preserving (verified by the
existing endpoint integration suite still passing).

## Testing

Testcontainers Postgres, no build tag, standard CI.

- **`TestReaperFailsStuckRuns`:** seed four runs on a thread —
  1. `in_progress` with `started_at = now() - 40m` (stale) → must become `failed`,
     with a `run.failed` row in `outbox`.
  2. `queued` with `created_at = now() - 20m` (stale) → must become `failed` +
     `run.failed` outbox row.
  3. `in_progress` with `started_at = now() - 2m` (fresh, mid-recovery window) →
     must stay `in_progress`, **untouched** (proves the reaper doesn't preempt
     JetStream recovery — the load-bearing assertion).
  4. `queued` with `created_at = now() - 1m` (fresh) → must stay `queued`.
  Run one `reapOnce(ctx)` tick; assert all four outcomes + the outbox rows.
- **`TestReaperIdempotent`:** a second tick over the now-`failed` runs does nothing
  (they're terminal; the `WHERE status IN ('in_progress','queued')` excludes them)
  — no duplicate `run.failed` events.
- **`eventstore` extraction:** the full `controlplane/endpoints` integration suite
  still passes (behavior-preserving), plus a direct `eventstore.Append` unit/
  integration test (appends an event + outbox row in a tx).
- Full `go test ./controlplane/... -count=1` green.

## Files

- `controlplane/eventstore/eventstore.go` (new) — `Event` + `Append`.
- `controlplane/endpoints/runtime.go` (modify) — `writeTx` uses `eventstore.Append`;
  `Event` re-uses `eventstore.Event`.
- `controlplane/reaper/reaper.go` (new) — `RunReaper`, `New…`, `Start`/`Stop`,
  `reapOnce`.
- `controlplane/server/server.go` (modify) — construct + wire the reaper (fields,
  `New`, `Run`, `Shutdown`), gated like the other background workers.
- Tests: `controlplane/reaper/reaper_integration_test.go`,
  `controlplane/eventstore/eventstore_test.go`.

## Known limitation — `started_at` threshold vs long-running graphs (recorded)

`in_progress` staleness is measured from `started_at`, which assumes a run
*completes* within `InProgressStaleAfter` (30 min). That holds for this slice (the
counter graph is instant) and for anything shorter than the threshold. But a real,
long-running graph engine (a separate future slice) could legitimately execute
past 30 min and be **falsely reaped**. When the real executor lands, `in_progress`
detection must switch from absolute `started_at` to a **liveness signal** — e.g.
the timestamp of the run's most recent `execution_history` node event, or a
run-level heartbeat the worker refreshes — so long-but-live runs aren't reaped.
This is a deliberate follow-up bound to the graph-engine work, called out here so
the 30-min threshold isn't mistaken for a permanent answer.

## Out of scope (recorded)

- **Recovery / re-dispatch** of stuck runs (requeue + re-publish
  `worker.graph.execute` to resume from checkpoint, with a bounded attempt count).
  Chosen against for this slice — the reaper fails; recovery is a deliberate later
  enhancement.
- The advisory-consumer backstop for the worker-crashes-on-its-own-last-delivery
  edge (a different mechanism; still deferred).
- `requires_action` runs (HITL interrupts) — those are legitimately non-terminal
  awaiting input, NOT stuck; the reaper ignores them (only `in_progress`/`queued`).
