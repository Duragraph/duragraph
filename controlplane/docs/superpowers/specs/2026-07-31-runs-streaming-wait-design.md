# Pass D — runs streaming (SSE) + wait

**Date:** 2026-07-31
**Branch:** `feat/controlplane-runs-streaming` (off `main`)
**Status:** approved design, pending implementation plan

## Context

Runs execute end-to-end (worker path + reaper merged), but a client can't yet
**observe** or **await** a run. This slice adds the streaming/observability
endpoints: SSE streams of a run's events and blocking wait-until-terminal. The
hard NATS plumbing already exists — the relay publishes `run.*`/`execution.*` to
JetStream, and `nats.Subscriber` (core-NATS ephemeral tail: `Subscribe(ctx,
subject) → <-chan *SubscriptionMsg`) is built. This slice wires that subscriber
into the endpoints and gives the `sse`/`wait` stubs bespoke handler bodies.

Source of truth: `endpoint-queries.d2` (runs `stream_*`/`join`/`wait` blocks),
`nats.d2` (RUNS/EXECUTION subjects), `duragraph-latest.yaml` (content types).

### Scope

**In (6 endpoints, all `custom` — SSE/wait can't be generator-fillable):**
- SSE (`text/event-stream`): `stream_per_run` (`GET /threads/{id}/runs/{rid}/stream`),
  `stream_thread` (`GET /threads/{id}/stream`), `create_and_stream`
  (`POST /threads/{id}/runs/stream`), `stateless_stream` (`POST /runs/stream`).
- wait (`application/json`, block-to-terminal): `join`
  (`GET /threads/{id}/runs/{rid}/join`), `wait` (`POST /runs/wait`).

**Out (separate passes):** `cancel`, `resume` (HITL), `batch` (event-sourced
writes, no SSE); LangGraph SDK stream-mode format (see below).

### Frame format: thin passthrough (not LangGraph stream modes)

Frames carry the real events we emit — `run.started`, `execution.node_*`,
`run.completed`/`failed`/`cancelled` — as `event: <event_type>\ndata:
<payload>\n\n`. This proves the full pipe (worker → outbox → relay → NATS → SSE →
client) with real data. It is **not** the LangGraph SDK stream-mode format
(`values`/`updates`/`messages`) — those require real graph-state deltas the counter
executor can't produce; that mapping is a refinement bound to the real graph
engine.

## Architecture

### Wiring the subscriber into endpoints

`endpoints.Server` gains a `Subscriber *nats.Subscriber` field (nil when NATS is
disabled). `server.go` constructs it from the same `*nats.Conn` used for the relay
(`nats.NewSubscriberFromConn(nc)`) and sets it on the `endpoints.Server`. SSE/wait
handlers require it: when `s.Subscriber == nil`, they return `503` (streaming needs
NATS).

The relay's published subjects are `duragraph.runs.>` and `duragraph.executions.>`
(envelope body: `{event_id, aggregate_type, aggregate_id, event_type, payload,
metadata}` — `aggregate_id` is the run id). A per-run handler subscribes to both
and filters received messages by `aggregate_id == run_id`.

### SSE handler pattern — lossless (the central correctness design)

Core-NATS subscription is live-only (no replay), and counter runs finish in
milliseconds, so a naive "subscribe and stream" shows an existing run nothing. The
handler is therefore **subscribe-first, then catch up, then go live, deduping**:

1. **Subscribe first** to `duragraph.runs.>` + `duragraph.executions.>` (start
   buffering live events *before* the DB read, so nothing slips through the gap
   between read and subscribe).
2. **Catch up** — `SELECT event_id, event_type, payload FROM events WHERE
   aggregate_id = $run_id ORDER BY event_version` — emit each as a frame, recording
   the set of emitted `event_id`s.
3. **Go live** — for each buffered/incoming NATS message: parse the envelope, skip
   unless `aggregate_id == run_id`, skip if `event_id` already emitted (dedup),
   else emit.
4. **Close** on the run's terminal event (`event_type ∈ {run.completed,
   run.failed, run.cancelled}`) or client disconnect
   (`<-c.Request().Context().Done()`). This per-run terminal close applies to
   `stream_per_run`, `create_and_stream`, `stateless_stream`. **`stream_thread`
   closes on client disconnect only** — a thread is a live feed that can have many
   runs (and new ones start over time), so no single run's terminal ends it.

Frame write (Echo): set headers `Content-Type: text/event-stream`, `Cache-Control:
no-cache`, `Connection: keep-alive`; write `event: <type>\ndata: <json>\n\n`; then
`c.Response().Flush()`. The subscription's `ctx` is `c.Request().Context()`, so a
disconnect cancels it and `Subscriber` unsubscribes + closes the channel.

- **`stream_thread`** resolves the thread's run ids first (`SELECT id FROM runs
  WHERE thread_id=$tid`) and filters by `aggregate_id ∈` that set (catch-up unions
  each run's events; a thread with no runs yet just streams live as runs appear).
- **`create_and_stream` / `stateless_stream`** first create the run (reusing the
  existing `run.created` write — `writeTx` + the runs projection), then run the SSE
  pattern on the new run id. (Catch-up is near-empty since the run was just created;
  events arrive live as the worker executes.)

### wait / join — block until terminal

Same subscribe-first discipline so the terminal event isn't missed in the gap:

1. Subscribe first to `duragraph.runs.>` filtered to the run.
2. `SELECT status FROM runs WHERE id=$rid` — if already terminal
   (`completed`/`failed`/`cancelled`), skip waiting.
3. Otherwise block on the subscription until a terminal `run.*` event for the run
   (or a request-context/timeout deadline).
4. `SELECT *` the final run and return it as JSON (the `runRow.toAPI()` shape).

`wait` (`POST /runs/wait`) creates a stateless run first, then waits.

## Error handling

- `s.Subscriber == nil` (NATS off) → 503.
- Run/thread not found → 404 (validate with a `SELECT` before streaming/waiting).
- Bind failure on create-variants → 400.
- SSE: on subscribe error → 500 before headers; after headers are sent, a mid-stream
  error is logged and the stream closes (can't change the status code).
- wait: request-context cancel/timeout → return whatever the run's current state is,
  or 504 on an explicit deadline (design: return current run state on ctx done).

## Testing

Testcontainers Postgres + embedded in-process NATS, no build tag, standard CI.

- **`TestStreamPerRunCatchUpAndLive`:** seed a run + insert two events (catch-up)
  into `events`; start the SSE handler against an `httptest` server; read the SSE
  response; assert the two catch-up frames arrive; then publish a live
  `execution.node_completed` + a terminal `run.completed` to NATS; assert both
  arrive (live, deduped — the terminal closes the stream). Confirms catch-up + live
  + dedup + terminal-close.
- **`TestStreamDedup`:** an event present in BOTH the catch-up read and the live
  feed (same `event_id`) is emitted once.
- **`TestJoinReturnsOnTerminal`:** seed an `in_progress` run; in a goroutine publish
  `run.completed` after a short delay; assert `join` blocks then returns the run
  with terminal status. And: an already-terminal run returns immediately.
- **`TestCreateAndStream`:** `POST /threads/{id}/runs/stream` creates the run
  (assert a `runs` row + `run.created` in outbox) and opens an SSE stream; publish
  the run's events; assert frames arrive.
- **`TestStreamRequiresNATS`:** `Subscriber == nil` → 503.
- One lighter **end-to-end** reusing the worker harness: create-and-stream a run,
  let the in-process worker execute the counter graph, assert the SSE client
  receives `run.started` → `node_completed`×2 → `run.completed`.
- Full `go test ./controlplane/... -count=1` green.

## Files

- `controlplane/endpoints/runtime.go` (modify) — add `Subscriber *nats.Subscriber`
  to `Server`. (Import `controlplane/nats`.)
- `controlplane/gen/endpoints.yaml` (modify) — mark the 6 runs streaming/wait
  endpoints `custom`.
- `controlplane/endpoints/runs_gen.go` (regenerate) — routes-only for those 6.
- `controlplane/endpoints/runs_stream.go` (new) — the SSE + wait handlers + a shared
  `streamRun`/`catchUp`/`isTerminal` helper set.
- `controlplane/server/server.go` (modify) — construct `nats.NewSubscriberFromConn`
  and set `ep.Subscriber`; ensure the subscriber's conn drains on shutdown.
- Tests: `controlplane/endpoints/runs_stream_integration_test.go`.

## Out of scope (recorded)

- **LangGraph SDK stream-mode format** (`values`/`updates`/`messages`/`events`) —
  needs the real graph engine's state semantics; thin passthrough now.
- **`cancel` / `resume` / `batch`** — separate runs-writes pass.
- **Reconnect / Last-Event-ID resume** of a dropped SSE connection — clients
  reconnect and catch-up replays from the `events` table, but a formal
  `Last-Event-ID` cursor is not implemented this slice.
- **JetStream durable consumer** for SSE — core-NATS ephemeral is correct for a
  live client; durability isn't needed (catch-up covers the persisted history).
