# Worker Reliability Follow-ups (INF-1 + INF-2) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the two reliability gaps from PR #217's review — INF-2 (relay shutdown data race) and INF-1 (exhausted-redelivery + graph-error must become `run.failed`, not silent drops).

**Architecture:** INF-2 replaces relay's cross-goroutine listener-conn `Close()` with context cancellation so the conn is only ever touched by its owning goroutine. INF-1 gives the worker executor an error return (behind an `Executor` interface) and makes the runner escalate a poison run or an exhausted redelivery to `run.failed` + Ack, wiring in the previously-dead `Client.RunFailed`.

**Tech Stack:** Go 1.25, pgx v5, NATS JetStream (`nats.go` jetstream + embedded server for tests), Echo, testcontainers Postgres.

## Global Constraints

- Design doc: `controlplane/docs/superpowers/specs/2026-07-26-worker-reliability-inf1-inf2-design.md` — the contract; this plan implements it.
- Two commits, one PR (branch `feat/controlplane-reliability`, stacked on `feat/controlplane-worker-execution`).
- Integration tests use testcontainers + embedded NATS, no build tag, run in standard CI. Never weaken an assertion to pass; if behavior is wrong, fix the code.
- The reliability-critical checks MUST run under `-race` and pass with no data race (INF-2's whole point).
- `runs` has NO `lease_expires_at`; fencing is `lease_epoch` only. `Client.RunFailed` is epoch-fenced.
- `execution_history.node_type` ∈ {start,end,llm,tool,conditional}; the counter/stub nodes use `tool`.
- Never hand-edit `*_gen.go` / go.mod / go.sum. Conventional commits. No PR/push/merge without explicit approval.
- Run all commands from the worktree root `~/worktrees/duragraph/feat/controlplane-server` (the reliability branch is checked out there).

---

## File Structure

- `controlplane/nats/relay.go` (modify) — context-cancel shutdown; remove listener-conn sharing.
- `controlplane/nats/consumers.go` (modify) — `GraphExecutorMaxDeliver` const.
- `controlplane/worker/executor.go` (modify) — `Executor` interface + `Run` error return.
- `controlplane/worker/runner.go` (modify) — `ex`/`cl` become interfaces, `ProcessOne` returns epoch, `ackDecision` helper, escalation + graph-error wiring, `MaxDeliver` field.
- `cmd/worker/main.go` (modify) — pass `nats.GraphExecutorMaxDeliver`.
- Tests: `controlplane/nats/nats_integration_test.go` / `run_processor_test.go` area (relay -race), `controlplane/worker/*_test.go`.

---

## Task 1: INF-2 — relay shutdown without cross-goroutine conn close

**Files:**
- Modify: `controlplane/nats/relay.go`
- Test: the relay `-race` tests already exist (`TestRelayStop` in nats package; `TestServer_Relay_Wired` in server package) — this task makes them pass under `-race`.

**Interfaces:**
- Consumes: existing `Relay` (`Start`, `Stop`, `listenLoop`, `stopCh`, `shouldStop`, `sleepOrStop`).
- Produces: same public API (`Start(ctx) error`, `Stop()`), race-free. Removes `listener`, `listenerMu`, `setListener`, `clearListener`.

- [ ] **Step 1: Establish the failing (racy) baseline**

Run: `go test ./controlplane/nats/ -run TestRelayStop -race -count=1 2>&1 | tail -20`
Expected: FAIL with `WARNING: DATA RACE` (Stop's `conn.Close` vs listenLoop's `WaitForNotification` on the same `*pgx.Conn`). Record that this is the baseline the fix must clear. (If your environment somehow doesn't reproduce it in one run, use `-count=10`.)

- [ ] **Step 2: Add a cancel field; drive shutdown by context**

In `controlplane/nats/relay.go`, change the `Relay` struct: remove `listenerMu sync.Mutex` and `listener *pgx.Conn`; add a cancel func guarded for set-once:

```go
type Relay struct {
	drain       *OutboxDrain
	publisher   *Publisher
	listenerDSN string
	safetyNet   time.Duration
	batchSize   int
	stopCh      chan struct{}

	cancelMu sync.Mutex
	cancel   context.CancelFunc // set by Start, called by Stop to unblock the wait
}
```

(Keep the `sync` import — it's still used by `cancelMu`. Remove `setListener`/`clearListener` methods entirely.)

- [ ] **Step 3: Make `Start` own a cancelable context and close its own conn**

In `Start`, derive a cancelable context once, store its cancel, and use it everywhere `ctx` was used for connect/LISTEN/listenLoop. Replace the `r.setListener(conn)` / `r.clearListener()` calls (they are deleted). The existing `_ = conn.Close(...)` calls in Start stay — they run in Start's own goroutine, which is now the ONLY place the conn is closed.

At the top of `Start`, after the arg checks:

```go
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	r.cancelMu.Lock()
	r.cancel = cancel
	r.cancelMu.Unlock()
```

Then use `runCtx` (not `ctx`) for: `r.shouldStop(runCtx)`, `pgx.Connect(runCtx, ...)`, `conn.Exec(runCtx, "LISTEN ...")`, `r.sleepOrStop(runCtx, ...)`, `r.processOutbox(runCtx)`, and `r.listenLoop(runCtx, conn)`. Delete the `r.setListener(conn)` line and the two `r.clearListener()` lines.

- [ ] **Step 4: `listenLoop` — distinguish Stop-cancel from parent-cancel**

In `listenLoop`, the `WaitForNotification` now unblocks via `runCtx` cancellation. Update the `context.Canceled` case so a Stop (stopCh closed) returns the clean `errStopRequested` while a parent-ctx cancel returns the ctx error:

```go
		case errors.Is(err, context.Canceled):
			select {
			case <-r.stopCh:
				return errStopRequested // Stop() → clean shutdown (Start returns nil)
			default:
				return ctx.Err() // parent ctx canceled
			}
```

(The passed `ctx` here is `runCtx`; `ctx.Err()` is `context.Canceled`.)

- [ ] **Step 5: `Stop` cancels instead of closing the conn**

Replace the body of `Stop` so it closes `stopCh` and calls the stored cancel — and never touches the conn:

```go
// Stop signals Start to exit and cancels the run context so any in-flight
// WaitForNotification unblocks immediately. The listener conn is closed only by
// Start's own goroutine, so there is no cross-goroutine use of the conn.
// Idempotent.
func (r *Relay) Stop() {
	select {
	case <-r.stopCh:
		return // already stopped
	default:
		close(r.stopCh)
	}
	r.cancelMu.Lock()
	cancel := r.cancel
	r.cancelMu.Unlock()
	if cancel != nil {
		cancel()
	}
}
```

- [ ] **Step 6: Build + vet**

Run: `go build ./controlplane/... && go vet ./controlplane/nats/`
Expected: clean (no unused `listener`/`listenerMu`, imports still satisfied).

- [ ] **Step 7: The race is gone (the whole point)**

Run: `go test ./controlplane/nats/ -run TestRelayStop -race -count=10`
Expected: PASS, no `WARNING: DATA RACE`.

Run: `go test ./controlplane/server/ -run TestServer_Relay_Wired -race -count=5`
Expected: PASS, no data race.

- [ ] **Step 8: Shutdown still works + relay still drains**

Run: `go test ./controlplane/nats/ -count=1` and `go test ./controlplane/server/ -count=1`
Expected: PASS — relay still shuts down promptly on Stop, still drains on NOTIFY, reconnect path intact.

- [ ] **Step 9: Commit**

```bash
git add controlplane/nats/relay.go
git commit -m "fix(controlplane): relay shutdown cancels context instead of closing listener conn cross-goroutine (fixes -race)"
```

---

## Task 2: INF-1 — dead-letter escalation + graph-error → run.failed

**Files:**
- Modify: `controlplane/nats/consumers.go`
- Modify: `controlplane/worker/executor.go`
- Modify: `controlplane/worker/runner.go`
- Modify: `cmd/worker/main.go`
- Test: `controlplane/worker/runner_ackdecision_test.go` (create), `controlplane/worker/execution_integration_test.go` (extend)

**Interfaces:**
- Consumes: `Client` (RunStarted/LatestCheckpoint/WriteCheckpoint/NodeCompleted/RunCompleted/RunFailed), `CounterExecutor`, `jetstream.Msg` metadata.
- Produces:
  - `nats.GraphExecutorMaxDeliver` (int const, = 5).
  - `worker.Executor` interface (`Nodes() []string`, `Run(step int, state map[string]int) (map[string]int, error)`); `CounterExecutor` satisfies it.
  - `worker.runClient` interface (the subset of `*Client` the runner uses); `*Client` satisfies it.
  - `Runner.MaxDeliver int`; `NewRunner(js, cl, ex, maxDeliver)` OR a `MaxDeliver` field set post-construction (choose one, below).
  - `Runner.ProcessOne(ctx, cmd) (acked bool, epoch int, err error)`.
  - `ackDecision(acked bool, epoch, numDelivered, maxDeliver int) decision` (pure).

- [ ] **Step 1: Write the failing `ackDecision` unit test**

Create `controlplane/worker/runner_ackdecision_test.go` (internal `package worker` test so it can see the unexported helper):

```go
package worker

import "testing"

func TestAckDecision(t *testing.T) {
	const max = 5
	cases := []struct {
		name         string
		acked        bool
		epoch        int
		numDelivered int
		want         decision
	}{
		{"success acks", true, 1, 3, decisionAck},
		{"transient below max naks", false, 1, 4, decisionNak},
		{"transient at max with lease escalates", false, 1, 5, decisionEscalate},
		{"transient past max with lease escalates", false, 2, 6, decisionEscalate},
		{"transient at max without lease just acks", false, 0, 5, decisionAck},
		{"acked wins even at max", true, 0, 5, decisionAck},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ackDecision(c.acked, c.epoch, c.numDelivered, max); got != c.want {
				t.Errorf("ackDecision(%v,%d,%d,%d) = %v, want %v", c.acked, c.epoch, c.numDelivered, max, got, c.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run it — fails (undefined)**

Run: `go test ./controlplane/worker/ -run TestAckDecision -v`
Expected: FAIL — `undefined: ackDecision` / `decision`.

- [ ] **Step 3: Add the shared max-deliver const**

In `controlplane/nats/consumers.go`, add near the top (package level):

```go
// GraphExecutorMaxDeliver bounds worker.graph.execute redeliveries. Shared so the
// consumer config and the worker's dead-letter escalation threshold agree.
const GraphExecutorMaxDeliver = 5
```

Change the `graph-executor` spec to use it: `maxDeliver: GraphExecutorMaxDeliver` (replacing the literal `5`).

- [ ] **Step 4: Add the `ackDecision` helper**

In `controlplane/worker/runner.go` add:

```go
// decision is what Start does with a JetStream message after ProcessOne.
type decision int

const (
	decisionAck      decision = iota // Ack (success, terminal, or nothing to fail)
	decisionNak                      // Nak (transient, redeliver)
	decisionEscalate                 // RunFailed(epoch) then Ack (final delivery, still failing)
)

// ackDecision decides the post-ProcessOne action. A successful/terminal outcome
// acks. A transient failure naks UNTIL the final allowed delivery; on that final
// delivery it escalates to run.failed when the run was leased (epoch>0), else acks
// (the run was never leased — nothing to fail; see design doc's known limitation).
func ackDecision(acked bool, epoch, numDelivered, maxDeliver int) decision {
	if acked {
		return decisionAck
	}
	if numDelivered >= maxDeliver {
		if epoch > 0 {
			return decisionEscalate
		}
		return decisionAck
	}
	return decisionNak
}
```

- [ ] **Step 4b: Run the unit test — passes**

Run: `go test ./controlplane/worker/ -run TestAckDecision -v`
Expected: PASS (all 6 subcases).

- [ ] **Step 5: `Executor` interface + error return**

In `controlplane/worker/executor.go`:

```go
// Executor is a graph the runner can execute step by step. Run returns the new
// state, or a non-nil error for a DETERMINISTIC (poison) failure — the runner
// turns that into run.failed and does NOT redeliver.
type Executor interface {
	Nodes() []string
	Run(step int, state map[string]int) (map[string]int, error)
}
```

Change `CounterExecutor.Run` to return `(map[string]int, error)` (always `nil` error):

```go
func (CounterExecutor) Run(step int, state map[string]int) (map[string]int, error) {
	out := map[string]int{}
	for k, v := range state {
		out[k] = v
	}
	out["count"] = step + 1
	return out, nil
}
```

- [ ] **Step 6: Runner uses interfaces, MaxDeliver, epoch return, graph-error wiring**

In `controlplane/worker/runner.go`:

1. Add the client interface (methods the runner calls; match `*Client`'s signatures exactly — verify against `controlplane/worker/client.go`):

```go
// runClient is the subset of *Client the runner uses; an interface so tests can
// inject failures. *Client satisfies it.
type runClient interface {
	RunStarted(ctx context.Context, runID uuid.UUID) (int, error)
	LatestCheckpoint(ctx context.Context, threadID, runID uuid.UUID) (int, []byte, bool, error)
	WriteCheckpoint(ctx context.Context, threadID, runID uuid.UUID, epoch, version int, state []byte) error
	NodeCompleted(ctx context.Context, runID uuid.UUID, epoch int, nodeID, nodeType string) error
	RunCompleted(ctx context.Context, runID uuid.UUID, epoch int) error
	RunFailed(ctx context.Context, runID uuid.UUID, epoch int, reason string) error
}
```

(If any signature differs from `client.go`, use `client.go`'s actual signature — it is the source of truth.)

2. Change the `Runner` struct: `cl runClient` (was `*Client`), `ex Executor` (was `CounterExecutor`), add `MaxDeliver int`. Update `NewRunner` to accept `ex Executor` and a `maxDeliver int` (keep `StopAfterNode` default -1):

```go
func NewRunner(js jetstream.JetStream, cl runClient, ex Executor, maxDeliver int) *Runner {
	return &Runner{js: js, cl: cl, ex: ex, MaxDeliver: maxDeliver, StopAfterNode: -1}
}
```

3. `ProcessOne` returns `(acked bool, epoch int, err error)`. Thread the leased `epoch` out (0 when `RunStarted` returned `ErrStaleLease` / never leased). At the graph-run step, handle a deterministic executor error:

```go
	state, rerr := r.ex.Run(step, state)
	if rerr != nil {
		// Poison run — record run.failed and stop; do NOT redeliver.
		if ferr := r.cl.RunFailed(ctx, cmd.RunID, epoch, rerr.Error()); ferr != nil {
			if errors.Is(ferr, ErrStaleLease) {
				return true, epoch, nil // superseded — ack
			}
			return false, epoch, ferr // transient failure to record — nak, retry
		}
		return true, epoch, nil // failure recorded — ack
	}
```

Update every `return` in `ProcessOne` to the 3-value form (add `epoch`; it is 0 before `RunStarted` succeeds, then the leased value).

4. In `Start`, use `ackDecision` + escalation. Replace the ack/nak block:

```go
	acked, epoch, perr := r.ProcessOne(ctx, cmd)
	meta, _ := msg.Metadata()
	nd := 0
	if meta != nil {
		nd = int(meta.NumDelivered)
	}
	switch ackDecision(acked, epoch, nd, r.MaxDeliver) {
	case decisionEscalate:
		reason := "max deliveries exceeded"
		if perr != nil {
			reason += ": " + perr.Error()
		}
		if ferr := r.cl.RunFailed(ctx, cmd.RunID, epoch, reason); ferr != nil {
			slog.Warn("runner: escalation RunFailed failed", "run_id", cmd.RunID, "err", ferr)
		}
		_ = msg.Ack()
	case decisionNak:
		_ = msg.Nak()
	default: // decisionAck
		_ = msg.Ack()
	}
```

(Keep the existing malformed-command → Ack path and the `msg.InProgress()` call before work.)

- [ ] **Step 7: `cmd/worker` passes the shared const**

In `cmd/worker/main.go`, update the `NewRunner(...)` call to pass `nats.GraphExecutorMaxDeliver` as the new `maxDeliver` arg. (`cmd/worker` already imports the nats package.)

- [ ] **Step 8: Build + the existing worker/nats/server suites still pass**

Run: `go build ./... && go vet ./controlplane/... ./cmd/...`
Expected: clean. Update ALL `NewRunner` call sites to the new 4-arg signature (verified callers): `cmd/worker/main.go:110` (production — pass `nats.GraphExecutorMaxDeliver`), and `controlplane/worker/execution_integration_test.go` lines **56, 109, 134** (the end-to-end + durable-resume tests — these call `ProcessOne` directly and don't exercise escalation, so any `maxDeliver` is fine; pass `nats.GraphExecutorMaxDeliver` for consistency). Also update any `ProcessOne` result destructuring to the new `(acked, epoch, err)` 3-value form. Fix anything that doesn't compile.

Run: `go test ./controlplane/worker/ -run 'TestExecuteRunEndToEnd|TestDurableResume' -race -count=1 -v`
Expected: PASS — the durable-resume proof still holds (node A once); the counter path is unaffected by the error return (always nil).

- [ ] **Step 9: Write the graph-error integration test (INF-1b)**

Add to `controlplane/worker/execution_integration_test.go` a `failingExecutor` and a test. Add `"fmt"` to that file's imports (it is not currently imported). The stub errors at node B:

```go
type failingExecutor struct{}

func (failingExecutor) Nodes() []string { return []string{"A", "B"} }
func (failingExecutor) Run(step int, state map[string]int) (map[string]int, error) {
	if step == 1 {
		return nil, fmt.Errorf("boom at node B")
	}
	out := map[string]int{}
	for k, v := range state {
		out[k] = v
	}
	out["count"] = step + 1
	return out, nil
}
```

Test `TestGraphErrorMarksRunFailed`: seed a queued run on a thread; build a `Runner` with `failingExecutor{}` and the real client; call `ProcessOne` (or drive one command through `Start`) once; assert:
- returned `acked == true` (poison → ack, no redelivery),
- run status ends `failed` (`SELECT status FROM runs WHERE id=$rid`),
- node A ran exactly once (`execution_history` node_id='A' count == 1),
- a `run.failed` event is in the outbox.

- [ ] **Step 10: Write the escalation integration test (INF-1a)**

Add a stub `runClient` that leases successfully (returns epoch 1) then returns a transient error from `WriteCheckpoint`, plus a `RunFailed` spy. Build a `Runner` with `MaxDeliver: 1` and this stub; drive the escalation path (`ackDecision(acked=false, epoch=1, numDelivered=1, maxDeliver=1)` → escalate) by invoking the same `Start`-branch logic (either through `Start` with an injected message whose `NumDelivered=1`, or by unit-testing the escalation wiring with the stub client). Assert `RunFailed` was called with `epoch=1` and the message was Acked (not Nak'd).

(If driving a real `jetstream.Msg` with `NumDelivered=1` is impractical in-test, assert the wiring at the seam: given `ackDecision` returns `decisionEscalate`, `Start` calls `cl.RunFailed` then Acks. A small test that calls the extracted escalation branch with the stub client + a fake/asserted ack is acceptable — the pure `ackDecision` matrix (Step 1) already covers the decision itself.)

- [ ] **Step 11: Full regression under race for the reliability paths**

Run: `go test ./controlplane/worker/ -race -count=1`
Run: `go test ./controlplane/... -count=1`
Expected: all PASS.

- [ ] **Step 12: Commit**

```bash
git add controlplane/nats/consumers.go controlplane/worker/executor.go \
  controlplane/worker/runner.go cmd/worker/main.go \
  controlplane/worker/runner_ackdecision_test.go controlplane/worker/execution_integration_test.go
git commit -m "feat(worker): dead-letter escalation + graph-error → run.failed (wire Client.RunFailed)"
```

---

## Self-Review

**Spec coverage:**
- INF-2 context-cancel shutdown + remove listener sharing → Task 1. ✓
- INF-1b Executor interface + error → run.failed → Task 2 (steps 5,6,9). ✓
- INF-1a epoch return + ackDecision + escalation + shared const → Task 2 (steps 1,3,4,6,10). ✓
- runClient interface for testability → Task 2 (step 6). ✓
- Known limitation (never-leased → ack, not fail) → encoded in `ackDecision` (epoch=0 at max → decisionAck) + tested (Step 1 "without lease just acks"). ✓

**Placeholder scan:** no TBD. The one soft spot is Step 10's escalation integration — the plan gives an explicit fallback (seam test with the stub client) because driving a real `jetstream.Msg` with a forced `NumDelivered` is impractical; the pure `ackDecision` matrix (Step 1) is the primary decision coverage, and the seam test covers the `RunFailed`+Ack wiring. This is a deliberate, stated testing boundary, not a vague requirement.

**Type consistency:** `ackDecision(acked bool, epoch, numDelivered, maxDeliver int) decision` used identically in Step 1 (test), Step 4 (def), Step 6 (Start). `ProcessOne`'s new `(acked, epoch, err)` signature is threaded through all its returns (Step 6.3) and its callers (Step 8). `runClient` method set matches `client.go` (Step 6.1 says verify against source). `NewRunner(js, cl, ex, maxDeliver)` updated at its only production caller (`cmd/worker`, Step 7) and test callers (Step 8).

**Open risks for the implementer:**
- Task 1: after removing the listener sharing, confirm the reconnect path still closes its conn (the existing `_ = conn.Close(...)` lines in Start stay). Do not remove those.
- Task 1: `runCtx` must be used for `WaitForNotification`'s parent so Stop's cancel unblocks it; if the wait still derives from the raw `ctx`, Stop won't unblock and the test will hang.
- Task 2: `msg.Metadata()` returns `(*MessageMetadata, error)`; `NumDelivered` is `uint64` — cast to int. Guard the nil-metadata case (Step 6.4 does).

---

## Notes for the implementer

- Do NOT reintroduce any cross-goroutine use of the relay's listener conn — the whole INF-2 fix is that the conn is closed only by Start's own goroutine.
- The counter executor never errors; the graph-error path is exercised only by `failingExecutor` in tests. That is expected — the interface exists for the real graph engine (future).
- Do not push/PR/merge without explicit approval; this branch is stacked on the (open) PR #217 and will be rebased after it merges.
