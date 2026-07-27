# Worker reliability follow-ups — INF-1 (dead-letter + graph-error) and INF-2 (relay race)

**Date:** 2026-07-26
**Branch:** `feat/controlplane-reliability` (stacked on `feat/controlplane-worker-execution`; rebase onto main after PR #217 merges)
**Status:** approved design, pending implementation plan

## Context

The worker execution path (PR #217) shipped a thin counter-graph slice. Its final
review flagged two reliability gaps the user prioritized:

- **INF-1** — two legs of the stated reliability spine are unwired: (a) exhausting
  `max_deliver` silently drops the run (no `run.failed`); (b) a deterministic graph
  error has no `run.failed`→Ack path (`Client.RunFailed` is dead code; the counter
  executor can't error).
- **INF-2** — two pre-existing `relay.go` data races (only under `-race`; CI does
  not run `-race`). Root cause: `Relay.Stop()` closes the listener `pgx.Conn` from
  a different goroutine to unblock an in-flight `WaitForNotification`, but a
  `pgx.Conn` is not safe for concurrent `Close`+`Wait`. The existing `listenerMu`
  guards only the pointer, not the conn's internals.

Both are independent (different files/packages) and land as two commits on one
branch / one PR.

## INF-2 — relay shutdown without a cross-goroutine conn close

**File:** `controlplane/nats/relay.go`

Current shutdown (racy): `Start` records the listener conn via `setListener`;
`Stop()` reads it under `listenerMu` and calls `conn.Close()` from the caller's
goroutine while `listenLoop` is inside `conn.WaitForNotification` on the same conn.
The mutex protects the `r.listener` pointer but not the concurrent use of the conn
itself — hence the `-race` failures in `TestRelayStop` and `TestServer_Relay_Wired`.

**Fix:** unblock the wait via **context cancellation**, so the conn is only ever
touched by its owning goroutine.

- `Start(ctx)` derives `runCtx, cancel := context.WithCancel(ctx)` and stores
  `cancel` on the Relay (guarded or set-once before the goroutine can call Stop).
- `listenLoop` uses `runCtx` for `WaitForNotification` (keeping the existing
  per-iteration wait timeout as a child of `runCtx`). pgx returns from
  `WaitForNotification` when its context is canceled.
- `Stop()` closes `stopCh` (unchanged, idempotent) **and calls `cancel()`** —
  it no longer touches the conn at all.
- The **listener goroutine** (inside `Start`) closes its own conn on loop exit
  (the existing reconnect-loop close path + a final close when `runCtx` is done).
- Remove `setListener`, `clearListener`, `r.listener`, `listenerMu` — the conn
  never escapes its owning goroutine, so no shared-conn state remains.

**Done-criteria:** `go test ./controlplane/nats/ -run TestRelayStop -race -count=1`
and `go test ./controlplane/server/ -run TestServer_Relay_Wired -race -count=1`
pass with no data race; the relay still shuts down promptly (Stop unblocks the
wait within the loop's timeout, ideally immediately via cancel) and still drains
on notify during normal operation.

## INF-1 — dead-letter escalation + graph-error → run.failed

**Files:** `controlplane/worker/executor.go`, `controlplane/worker/runner.go`,
`controlplane/nats/consumers.go`.

### Shared max-deliver constant

`maxDeliver: 5` currently lives only on the `graph-executor` `consumerSpec`. Expose
it as a package const so the consumer config and the runner's escalation threshold
share one source of truth:

- `controlplane/nats/consumers.go`: `const GraphExecutorMaxDeliver = 5`, and the
  `graph-executor` spec uses `maxDeliver: GraphExecutorMaxDeliver`.
- The runner takes a `MaxDeliver int`; `cmd/worker` passes
  `nats.GraphExecutorMaxDeliver`; tests may set a small value.

### 1b — graph error → run.failed (Executor interface)

- `executor.go`: define
  ```go
  type Executor interface {
      Nodes() []string
      Run(step int, state map[string]int) (map[string]int, error)
  }
  ```
  `CounterExecutor.Run` returns `(…, nil)` (never errors). This is the seam a
  failing test stub plugs into.
- `runner.go`: `Runner.ex` becomes `Executor` (interface); `NewRunner(js, cl,
  ex Executor)`.
- `runner.go`: `Runner.cl` becomes a minimal `runClient` interface (the exact
  subset of `*Client` methods the runner calls: `RunStarted`, `LatestCheckpoint`,
  `WriteCheckpoint`, `NodeCompleted`, `RunCompleted`, `RunFailed`). The real
  `*Client` satisfies it; tests inject a stub that returns a transient error after
  a successful lease, so the 1a escalation path is deterministically testable.
  This mirrors `ex` becoming an interface and adds no production behavior change.
- In `ProcessOne`, `state, rerr := r.ex.Run(step, state)`. On `rerr != nil` (a
  deterministic graph error — a poison run, not transient): post
  `r.cl.RunFailed(ctx, cmd.RunID, epoch, rerr.Error())`, then return `acked=true`
  (do NOT redeliver a poison graph). If `RunFailed` itself returns `ErrStaleLease`
  (superseded) → `acked=true`; if it returns a transient error → `acked=false`
  (Nak — the failure record didn't persist, retry).

### 1a — max-deliver escalation → run.failed

- `ProcessOne` returns the **leased epoch** so `Start` can mark the run failed on
  the last delivery. New signature:
  `func (r *Runner) ProcessOne(ctx, cmd) (acked bool, epoch int, err error)`.
  `epoch` is the value from `RunStarted` (0 when the run was never leased — e.g.
  `RunStarted` returned `ErrStaleLease`, meaning the run is already terminal/gone).
- Extract the terminal decision into a **pure, unit-testable helper**:
  ```go
  // ackDecision decides what to do with a JetStream message after ProcessOne.
  //   acked            → ProcessOne's acked return
  //   epoch            → leased epoch (0 = never leased)
  //   numDelivered     → msg.Metadata().NumDelivered
  //   maxDeliver       → the consumer's max_deliver
  // returns one of: ackOnly | nak | escalateFail (RunFailed then Ack)
  func ackDecision(acked bool, epoch, numDelivered, maxDeliver int) decision
  ```
  Rules: `acked` → `ackOnly`. Else (transient) if `numDelivered >= maxDeliver` →
  `escalateFail` when `epoch > 0`, else `ackOnly` (nothing to fail — run gone).
  Else → `nak`.
- `Start`'s consume handler applies it: on `escalateFail`, best-effort
  `r.cl.RunFailed(ctx, runID, epoch, "max deliveries exceeded: "+err)` then
  `msg.Ack()`; on `nak`, `msg.Nak()`; on `ackOnly`, `msg.Ack()`.
- `Client.RunFailed` is now called from both the 1a and 1b paths (was dead code).

## Testing

Testcontainers Postgres + embedded NATS (no build tag; standard CI). No assertion
weakening.

- **INF-2:** `TestRelayStop` (nats) and `TestServer_Relay_Wired` (server) pass
  under `-race -count=1`. Add a focused assertion that `Stop()` returns promptly
  and a subsequent `Start` still works (no leaked/closed-twice conn).
- **INF-1b:** a `failingExecutor` stub whose `Run` errors at node B. Drive a real
  run through the runner; assert: run status ends `failed`, node A's
  `execution_history` row exists (A ran), the message is **acked** (no infinite
  redelivery), and `run.failed` is in the outbox.
- **INF-1a:** unit-test `ackDecision` across the matrix:
  `(acked=true,*)→ackOnly`; `(acked=false, epoch>0, numDelivered>=max)→escalateFail`;
  `(acked=false, epoch=0, numDelivered>=max)→ackOnly`;
  `(acked=false, numDelivered<max)→nak`. Plus one integration check that the
  escalation path actually calls `RunFailed` + acks (e.g. a runner with
  `MaxDeliver=1` and an injected transient failure after lease → run ends `failed`
  via escalation on the first/last delivery).
- Full `go test ./controlplane/... -count=1` green.

## Files touched

- `controlplane/nats/relay.go` — context-cancel shutdown; drop listener-conn sharing.
- `controlplane/nats/consumers.go` — `GraphExecutorMaxDeliver` const.
- `controlplane/worker/executor.go` — `Executor` interface + error return.
- `controlplane/worker/runner.go` — interface field, epoch return, `ackDecision`
  helper, escalation + graph-error wiring, `MaxDeliver` field.
- `cmd/worker/main.go` — pass `nats.GraphExecutorMaxDeliver` to the runner.
- Tests: `controlplane/nats/*relay*_test.go` (or existing), `controlplane/worker/*_test.go`.

## Known limitation of worker-side escalation (recorded, not hidden)

Escalation to `run.failed` requires a **leased epoch**. Two exhaustion cases are
therefore NOT covered by this design, and both are acceptable-but-noted:

1. **Run not leased on the final delivery.** Escalation reads the epoch from the
   *current* delivery's `RunStarted`; if that call transient-fails on the final
   delivery, `epoch=0`, so escalation cannot call the epoch-fenced `RunFailed`. The
   message is Acked and the run is NOT marked failed. Two sub-cases:
   - **Never leased at all** (e.g. DB down the entire time): the run stays `queued`.
   - **Leased on an earlier delivery, then fails to re-lease on the final one**
     (e.g. a transient blip only on the last attempt): the run stays `in_progress`.
   Either way nothing re-dispatches it (run-processor only dispatches on
   `run.created`). This is strictly better than today (which silently drops every
   exhausted run) but leaves a stuck run in this narrow case. A lease-free "fail a
   stuck run" path (admin retry, or a queued/in_progress reaper) is a deliberate
   follow-up, not part of this slice.
2. **Worker crashes on its own last delivery**, before it can post `run.failed`.
   Same outcome as JetStream dropping the message. The advisory-consumer backstop
   (below) would close this; deferred.

## Out of scope (recorded)

- The advisory-consumer reaper backstop for the worker-crashes-on-its-last-delivery
  edge (chosen against: worker-side escalation covers the common case; the crash-on-
  last-delivery gap is an edge-of-edge, deferred).
- `input` seeding into the command; the checkpoint-before-node_completed
  observability gap (separate follow-ups from PR #217's review).
- A real graph engine (the `Executor` interface is added now, but the only
  implementation remains the counter + a test stub).
