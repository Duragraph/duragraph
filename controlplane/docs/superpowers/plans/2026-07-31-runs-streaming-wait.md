# Pass D — Runs Streaming (SSE) + Wait Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a client observe and await a run — SSE streams of a run's events and blocking wait-until-terminal — by wiring the existing `nats.Subscriber` into the endpoints and filling the `sse`/`wait` stubs with a lossless catch-up-then-live handler.

**Architecture:** `endpoints.Server` gains a `nats.Subscriber` (built in `server.go` from the relay's NATS conn). SSE handlers subscribe-first to `duragraph.runs.>`/`duragraph.executions.>`, replay the run's persisted events from the `events` table (catch-up), then stream live NATS messages filtered by `aggregate_id==run_id` and deduped by `event_id`, closing on the run's terminal event or client disconnect. Frames are thin passthrough (`event: <type>\ndata: <payload>`). Wait endpoints use the same subscribe-first discipline and return the final run as JSON.

**Tech Stack:** Go 1.25, Echo v4 (streaming + flush), pgx v5, NATS core subscription (`nats.Subscriber`), testcontainers Postgres + embedded NATS.

## Global Constraints

- Design doc: `controlplane/docs/superpowers/specs/2026-07-31-runs-streaming-wait-design.md` — the contract.
- **Lossless is the central property:** subscribe BEFORE the catch-up DB read; dedup live vs catch-up by `event_id`; a client connecting to an already-finished run must still receive its events (catch-up). The test that seeds catch-up events + publishes live + terminal is the guard.
- Frame format is thin passthrough: `event: <event_type>\ndata: <payload-json>\n\n`, flushed. NOT LangGraph stream-mode format.
- SSE endpoints require NATS: when `s.Subscriber == nil` → 503.
- Generated `*_gen.go` never hand-edited; the `custom` flag emits routes-only, bodies hand-written. Marking `sse`/`wait` endpoints `custom` must leave the impl runs endpoints (create/get/cancel) byte-identical after regen.
- **SSE tests MUST use `httptest.NewServer` (a real server), NOT `httptest.NewRecorder`** — the recorder buffers and doesn't support incremental flush/streaming.
- Integration tests: testcontainers Postgres + embedded in-process NATS, no build tag, standard CI. Never weaken assertions.
- Never hand-edit go.mod/go.sum. Conventional commits. No PR/push/merge without explicit approval.
- Run from the worktree root `~/worktrees/duragraph/feat/controlplane-server` (branch `feat/controlplane-runs-streaming`).

## File Structure

- `controlplane/endpoints/runtime.go` (modify) — add `Subscriber *nats.Subscriber` to `Server`.
- `controlplane/gen/endpoints.yaml` (modify) — mark the 6 runs `sse`/`wait` endpoints `custom`.
- `controlplane/endpoints/runs_gen.go` (regenerate) — routes-only for the 6.
- `controlplane/endpoints/runs_stream.go` (new) — SSE + wait handlers + shared helpers (`streamRun`, `catchUp`, `writeSSEFrame`, `isTerminalEvent`, `createRun`).
- `controlplane/server/server.go` (modify) — construct `nats.NewSubscriberFromConn` and set `ep.Subscriber`.
- Tests: `controlplane/endpoints/runs_stream_integration_test.go`.

---

## Task 1: Subscriber wiring + shared SSE plumbing + `stream_per_run`

**Files:**
- Modify: `controlplane/endpoints/runtime.go`, `controlplane/gen/endpoints.yaml`, `controlplane/server/server.go`
- Regenerate: `controlplane/endpoints/runs_gen.go`
- Create: `controlplane/endpoints/runs_stream.go`, `controlplane/endpoints/runs_stream_integration_test.go`

**Interfaces:**
- Produces: `Server.Subscriber *nats.Subscriber`; helpers `streamRun(c echo.Context, runIDs map[uuid.UUID]bool, closeOnTerminal bool) error`, `catchUp(ctx, aggIDs []uuid.UUID) ([]frame, error)`, `writeSSEFrame(c echo.Context, eventType string, payload []byte) error`, `isTerminalEvent(t string) bool`; handler `RunsStreamPerRun`.
- Consumes: `nats.Subscriber.Subscribe`, `nats.SubscriptionMsg`, the relay envelope (`{event_id, aggregate_id, event_type, payload}`).

- [ ] **Step 1: Write the failing tests (SSE read harness + stream_per_run)**

Create `controlplane/endpoints/runs_stream_integration_test.go`. It needs the shared `testPool` + an embedded NATS server. Add an embedded NATS to the endpoints package test setup if not present (mirror `controlplane/nats/nats_integration_test.go`'s embedded server; expose a package `testNATS *nats.Conn`). Then a real-server SSE read helper + the test:

```go
// readSSE connects to url, reads Server-Sent frames until `until` frames arrive or
// the deadline passes, and returns them. Uses a real HTTP client (streaming).
func readSSE(t *testing.T, url string, want int, deadline time.Duration) []sseFrame {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("sse connect: %v", err)
	}
	frames := make(chan sseFrame, 32)
	go func() {
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		var ev, data string
		for sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, "event: "):
				ev = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				data = strings.TrimPrefix(line, "data: ")
			case line == "":
				if ev != "" {
					frames <- sseFrame{Event: ev, Data: data}
					ev, data = "", ""
				}
			}
		}
	}()
	var got []sseFrame
	timeout := time.After(deadline)
	for len(got) < want {
		select {
		case f := <-frames:
			got = append(got, f)
		case <-timeout:
			return got
		}
	}
	return got
}

type sseFrame struct{ Event, Data string }
```

And the test (uses an `httptest.NewServer` around an Echo with the runs streaming routes, the shared `testPool`, and `testNATS` for the Subscriber):

```go
func TestStreamPerRunCatchUpAndLive(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE runs, events, outbox, event_streams, assistants CASCADE"); err != nil {
		t.Fatal(err)
	}
	// seed a run + two catch-up events (as if already emitted)
	rid := seedRunWithStream(t, ctx) // helper: assistant + run + event_stream, returns run uuid
	insertEvent(t, ctx, rid, 1, "run.started", `{"n":1}`)
	insertEvent(t, ctx, rid, 2, "execution.node_completed", `{"node":"A"}`)

	e := echo.New()
	s := &Server{Tenant: testPool, Subscriber: nats.NewSubscriberFromConn(testNATS)}
	s.RegisterRuns(e.Group("/api/v1"))
	srv := httptest.NewServer(e)
	defer srv.Close()

	url := srv.URL + "/api/v1/threads/" + uuid.Nil.String() + "/runs/" + rid.String() + "/stream"
	// Read in a goroutine; publish live + terminal after the stream is up.
	done := make(chan []sseFrame, 1)
	go func() { done <- readSSE(t, url, 4, 5*time.Second) }()
	time.Sleep(300 * time.Millisecond) // let subscribe+catchup run
	pub := nats.NewPublisher(mustJS(t)) // JetStream publisher on testNATS
	_ = pub.PublishWithID(ctx, nats.SubjectFor("execution.node_completed"), uuid.NewString(), envelopeFor(rid, "execution.node_completed", `{"node":"B"}`))
	_ = pub.PublishWithID(ctx, nats.SubjectFor("run.completed"), uuid.NewString(), envelopeFor(rid, "run.completed", `{}`))

	frames := <-done
	// Expect 4: 2 catch-up (run.started, node_completed A) + 2 live (node_completed B, run.completed)
	if len(frames) != 4 {
		t.Fatalf("want 4 frames, got %d: %+v", len(frames), frames)
	}
	if frames[0].Event != "run.started" || frames[3].Event != "run.completed" {
		t.Errorf("frame order wrong: %+v", frames)
	}
}
```

(Helpers `seedRunWithStream`, `insertEvent`, `envelopeFor`, `mustJS` are small; write them in the test file. `envelopeFor(rid, type, payload)` returns the relay envelope map `{event_id, aggregate_type:"Run", aggregate_id:rid, event_type, payload}`. The catch-up read keys on the `events` table, so `insertEvent` inserts into `events` with the given `event_version`.)

Also `TestStreamRequiresNATS`: a `Server{Tenant: testPool}` (nil Subscriber) → GET stream → 503. And `TestStreamDedup`: an event whose `event_id` is in BOTH the catch-up `events` row and the published live envelope is emitted once (assert frame count).

- [ ] **Step 2: Run — fails (handlers are stubs / no Subscriber field)**

Run: `go test ./controlplane/endpoints/ -run 'TestStreamPerRun|TestStreamRequiresNATS|TestStreamDedup' -v`
Expected: FAIL (compile: `Server` has no `Subscriber`; `RunsStreamPerRun` returns the sse stub `501`).

- [ ] **Step 3: Add the Subscriber field**

In `controlplane/endpoints/runtime.go`, add to `Server`:
```go
	// Subscriber tails NATS for SSE/wait endpoints. Nil when NATS is disabled
	// (those endpoints then return 503). Set by the server composition root.
	Subscriber *nats.Subscriber
```
Add the import `"github.com/duragraph/duragraph/controlplane/nats"`.

- [ ] **Step 4: Mark the 6 endpoints custom + regenerate**

In `controlplane/gen/endpoints.yaml`, add `custom: true` to the runs endpoints `join`, `stream_per_run`, `stream_thread`, `create_and_stream`, `stateless_stream`, `stateless_wait`. Run `go run ./controlplane/gen`. Confirm: `git diff --stat controlplane/endpoints/assistants_gen.go controlplane/endpoints/store_gen.go` empty (other groups untouched); `runs_gen.go` now routes `RunsJoin`/`RunsStreamPerRun`/`RunsStreamThread`/`RunsCreateAndStream`/`RunsStatelessStream`/`RunsStatelessWait` with no bodies, and the impl handlers (create/get/cancel) keep their bodies.

- [ ] **Step 5: Write the shared SSE plumbing + `stream_per_run`**

Create `controlplane/endpoints/runs_stream.go`:

```go
// Hand-written SSE + wait handlers for the runs group (routes generated into
// runs_gen.go via custom: true). Lossless: subscribe-first, replay persisted
// events (catch-up), then stream live NATS events deduped by event_id, closing on
// the run's terminal event or client disconnect. Thin passthrough frames.
package endpoints

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/duragraph/duragraph/controlplane/nats"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// relayEnvelope is the body the relay publishes (see nats/relay.go envelope()).
type relayEnvelope struct {
	EventID     string          `json:"event_id"`
	AggregateID string          `json:"aggregate_id"`
	EventType   string          `json:"event_type"`
	Payload     json.RawMessage `json:"payload"`
}

func isTerminalEvent(t string) bool {
	return t == "run.completed" || t == "run.failed" || t == "run.cancelled"
}

// RunsStreamPerRun streams one run's events. GET /threads/{id}/runs/{rid}/stream.
func (s *Server) RunsStreamPerRun(c echo.Context) error {
	if s.Subscriber == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "streaming requires NATS")
	}
	rid, err := uuid.Parse(c.Param("rid"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid rid")
	}
	// validate the run exists
	var exists bool
	if err := s.Tenant.QueryRow(c.Request().Context(),
		`SELECT true FROM runs WHERE id=$1`, rid).Scan(&exists); err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "run not found")
	}
	return s.streamRun(c, map[uuid.UUID]bool{rid: true}, true /*closeOnTerminal*/)
}

// streamRun runs the lossless catch-up-then-live loop for the given run id set.
// closeOnTerminal ends the stream when a terminal run.* event for a watched run
// arrives (per-run streams); false keeps it open until disconnect (thread feed).
func (s *Server) streamRun(c echo.Context, runIDs map[uuid.UUID]bool, closeOnTerminal bool) error {
	ctx := c.Request().Context()

	// 1. Subscribe FIRST (before catch-up) so nothing is missed in the gap.
	runsCh, err := s.Subscriber.Subscribe(ctx, "duragraph.runs.>")
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	execCh, err := s.Subscriber.Subscribe(ctx, "duragraph.executions.>")
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// SSE headers.
	h := c.Response().Header()
	h.Set(echo.HeaderContentType, "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	c.Response().WriteHeader(http.StatusOK)
	c.Response().Flush()

	seen := map[string]bool{} // event_id → emitted (dedup)

	// 2. Catch-up: replay persisted events for the watched runs, in order.
	ids := make([]uuid.UUID, 0, len(runIDs))
	for id := range runIDs {
		ids = append(ids, id)
	}
	rows, err := s.Tenant.Query(ctx, `
		SELECT event_id::text, event_type, payload FROM events
		WHERE aggregate_id = ANY($1) ORDER BY event_version`, ids)
	if err == nil {
		for rows.Next() {
			var eid, etype string
			var payload []byte
			if rows.Scan(&eid, &etype, &payload) == nil {
				seen[eid] = true
				if werr := writeSSEFrame(c, etype, payload); werr != nil {
					rows.Close()
					return nil // client gone
				}
				if closeOnTerminal && isTerminalEvent(etype) {
					rows.Close()
					return nil // run already finished — all events replayed
				}
			}
		}
		rows.Close()
	}

	// 3. Live: stream new events for the watched runs, deduped.
	for {
		var msg *nats.SubscriptionMsg
		select {
		case <-ctx.Done():
			return nil
		case msg = <-runsCh:
		case msg = <-execCh:
		}
		if msg == nil { // channel closed (ctx canceled)
			return nil
		}
		var env relayEnvelope
		if json.Unmarshal(msg.Payload, &env) != nil {
			continue
		}
		aid, err := uuid.Parse(env.AggregateID)
		if err != nil || !runIDs[aid] || seen[env.EventID] {
			continue
		}
		seen[env.EventID] = true
		if writeSSEFrame(c, env.EventType, env.Payload) != nil {
			return nil
		}
		if closeOnTerminal && isTerminalEvent(env.EventType) {
			return nil
		}
	}
}

func writeSSEFrame(c echo.Context, eventType string, payload []byte) error {
	if len(payload) == 0 {
		payload = []byte("{}")
	}
	if _, err := c.Response().Write([]byte("event: " + eventType + "\ndata: ")); err != nil {
		return err
	}
	if _, err := c.Response().Write(payload); err != nil {
		return err
	}
	if _, err := c.Response().Write([]byte("\n\n")); err != nil {
		return err
	}
	c.Response().Flush()
	return nil
}

var _ = context.Background // retained if unused after edits
```

- [ ] **Step 6: Wire the Subscriber in the server**

In `controlplane/server/server.go`, in the `cfg.NATSURL != ""` block where `nc, js` are obtained, after building the endpoints `ep`, set `ep.Subscriber = nats.NewSubscriberFromConn(nc)`. (The `ep` construction may be before the NATS block — if so, move the `ep.Subscriber` assignment to after `nc` exists, or set it when mounting. Ensure `ep.Subscriber` is nil when NATS is disabled.) The subscriber shares the relay's `nc`; it's drained on shutdown with the rest.

- [ ] **Step 7: Build + run the tests**

Run: `go build ./controlplane/...`
Run: `go test ./controlplane/endpoints/ -run 'TestStreamPerRun|TestStreamRequiresNATS|TestStreamDedup' -v -count=1`
Expected: PASS — catch-up (2) + live (2) frames, dedup, 503 without NATS.

Run: `go test ./controlplane/endpoints/ -count=1` (regression — the impl runs endpoints + all groups still green).

- [ ] **Step 8: Commit**

```bash
git add controlplane/endpoints/runtime.go controlplane/gen/endpoints.yaml \
  controlplane/endpoints/runs_gen.go controlplane/endpoints/runs_stream.go \
  controlplane/server/server.go controlplane/endpoints/runs_stream_integration_test.go
git commit -m "feat(controlplane): SSE stream_per_run + subscriber wiring + lossless catch-up/live plumbing"
```

---

## Task 2: `stream_thread` + `create_and_stream` + `stateless_stream`

**Files:**
- Modify: `controlplane/endpoints/runs_stream.go` (add handlers + `createRun` helper)
- Modify: `controlplane/endpoints/runs_stream_integration_test.go` (add tests)

**Interfaces:**
- Consumes: `streamRun`, `writeSSEFrame` (Task 1); `writeTx`, `mustJSON`, `asUUID` (existing runtime helpers); the runs projection SQL (mirrors `RunsCreateOnThread`/`RunsCreateStateless`).
- Produces: `RunsStreamThread`, `RunsCreateAndStream`, `RunsStatelessStream`; `createRun(ctx, threadID *uuid.UUID, req RunCreate…) (uuid.UUID, error)`.

- [ ] **Step 1: Write the failing tests**

Add `TestStreamThread` (seed a thread + 2 runs; catch-up replays both runs' events; `closeOnTerminal=false` — the stream stays open until the client disconnects; assert frames for both runs' seeded events arrive, then cancel the client) and `TestCreateAndStream` (`POST /threads/{id}/runs/stream` with a body → assert a `runs` row + `run.created` outbox row created, and the SSE stream opens; publish the run's events → frames arrive).

- [ ] **Step 2: Run — fails**

Run: `go test ./controlplane/endpoints/ -run 'TestStreamThread|TestCreateAndStream' -v`
Expected: FAIL (stub bodies).

- [ ] **Step 3: Add the handlers + createRun**

Append to `controlplane/endpoints/runs_stream.go`:
- `RunsStreamThread(c)`: parse `id` (thread), `SELECT id FROM runs WHERE thread_id=$1` → build the `runIDs` set (may be empty), `streamRun(c, ids, false /*stay open*/)`. (Catch-up over `aggregate_id = ANY(ids)` handles the union; an empty set streams live-only until a run appears.)
- `createRun(ctx, threadID *uuid.UUID, assistantID uuid.UUID, input, metadata []byte) (uuid.UUID, error)`: mirror `RunsCreateOnThread`'s create — `aggID := uuid.New()`; `writeTx(ctx, s.Tenant, []Event{{AggregateType:"Run", AggregateID: aggID, EventType:"run.created", Payload: …}}, projection)` where the projection does the `INSERT INTO runs (...)` (thread-scoped when `threadID != nil`, else stateless). Return `aggID`.
- `RunsCreateAndStream(c)`: bind the create request; `rid, err := s.createRun(ctx, &threadID, …)`; then `streamRun(c, map[uuid.UUID]bool{rid: true}, true)`.
- `RunsStatelessStream(c)`: same but `threadID = nil`.

(The generated `RunsCreateOnThread`/`RunsCreateStateless` keep their own inline create — `createRun` is shared only by the two custom stream-create handlers here, to avoid a generator change. Small, acceptable.)

- [ ] **Step 4: Build + tests + regression**

Run: `go build ./controlplane/... && go test ./controlplane/endpoints/ -run 'TestStreamThread|TestCreateAndStream' -v -count=1` → PASS.
Run: `go test ./controlplane/endpoints/ -count=1` → PASS.

- [ ] **Step 5: Commit**

```bash
git add controlplane/endpoints/runs_stream.go controlplane/endpoints/runs_stream_integration_test.go
git commit -m "feat(controlplane): SSE stream_thread + create_and_stream + stateless_stream"
```

---

## Task 3: `join` + `stateless_wait` + end-to-end

**Files:**
- Modify: `controlplane/endpoints/runs_stream.go` (add wait handlers)
- Modify: `controlplane/endpoints/runs_stream_integration_test.go` (add tests + e2e)

**Interfaces:**
- Consumes: `Subscriber.Subscribe`, `relayEnvelope`, `isTerminalEvent`, `runRow.toAPI()`, `createRun`.
- Produces: `RunsJoin`, `RunsStatelessWait`; `waitForRun(c, rid) (runRow, error)`.

- [ ] **Step 1: Write the failing tests**

`TestJoinReturnsOnTerminal`: seed an `in_progress` run; start `join` (GET) in a goroutine; after 300ms publish `run.completed` for the run; assert the handler returns 200 with the run whose status maps to a terminal API status. `TestJoinAlreadyTerminal`: seed a `completed` run → `join` returns immediately. `TestWaitRequiresNATS`: nil Subscriber → 503.

Plus **`TestStreamEndToEnd`** (the integration proof, reusing the worker harness pattern): stand up the worker endpoints + run-processor + an in-process worker + the SSE endpoints against shared testcontainers; `POST /threads/{id}/runs/stream`; assert the SSE client receives `run.started` → `execution.node_completed`×2 → `run.completed` as the counter graph executes. (If wiring the full worker harness into the endpoints test package is too heavy, place this test in `controlplane/worker` alongside the existing e2e harness and drive the SSE client there — note which package it lands in.)

- [ ] **Step 2: Run — fails**

Run: `go test ./controlplane/endpoints/ -run 'TestJoin|TestWait' -v` → FAIL.

- [ ] **Step 3: Add the wait handlers**

Append to `runs_stream.go`:
- `waitForRun(c echo.Context, rid uuid.UUID) error`: if `s.Subscriber == nil` → 503. Subscribe FIRST to `duragraph.runs.>`. `SELECT status FROM runs WHERE id=$1` — 404 if missing; if terminal, skip waiting. Else loop `select { <-ctx.Done(): return current run; case msg := <-ch: parse envelope; if aggregate_id==rid && isTerminalEvent(type) break }`. Then `SELECT *` the run into `runRow`, `return c.JSON(200, row.toAPI())`.
- `RunsJoin(c)`: parse `rid` → `waitForRun(c, rid)`.
- `RunsStatelessWait(c)`: bind create request → `rid, _ := createRun(ctx, nil, …)` → `waitForRun(c, rid)`.

- [ ] **Step 4: Build + tests + full regression (incl -race on the stream path)**

Run: `go build ./... && go test ./controlplane/endpoints/ -run 'TestJoin|TestWait|TestStream' -race -count=1 -v` → PASS.
Run: `go test ./controlplane/... -count=1` → whole rebuild green (incl. the e2e wherever it landed).

- [ ] **Step 5: Commit**

```bash
git add controlplane/endpoints/runs_stream.go controlplane/endpoints/runs_stream_integration_test.go
git commit -m "feat(controlplane): runs join + wait (block-until-terminal) + streaming e2e"
```

---

## Self-Review

**Spec coverage:** stream_per_run → T1; stream_thread/create_and_stream/stateless_stream → T2; join/stateless_wait → T3; subscriber wiring → T1 Step 6; lossless catch-up+live+dedup → `streamRun` (T1); thin passthrough frames → `writeSSEFrame`; 503-without-NATS → each handler; e2e → T3. ✓

**Placeholder scan:** the `var _ = context.Background` line in T1 Step 5 is a guard against an unused import during incremental edits — remove it if `context` ends up used (it is, via `ctx`); the implementer deletes it. T2/T3 handler bodies are specified by contract + reference the shared `streamRun`/`createRun`/`waitForRun` with exact SQL described — the non-obvious code (`streamRun`, `writeSSEFrame`, the SSE read harness) is given in full in T1; the reuse handlers are thin wrappers, appropriate to specify by contract.

**Type consistency:** `streamRun(c, map[uuid.UUID]bool, bool)`, `writeSSEFrame(c, string, []byte)`, `relayEnvelope{EventID,AggregateID,EventType,Payload}`, `isTerminalEvent(string) bool` used identically across tasks. Handler names match `pascal(runs)+pascal(name)`: `RunsStreamPerRun`/`RunsStreamThread`/`RunsCreateAndStream`/`RunsStatelessStream`/`RunsJoin`/`RunsStatelessWait`. `createRun` signature shared by T2/T3.

**Open risks for the implementer:**
- SSE tests MUST use `httptest.NewServer` (real server + flush), never `httptest.NewRecorder`. Stated in Global Constraints.
- The endpoints test package needs an embedded NATS conn (`testNATS`) — add it to the package's test setup (mirror the nats package) if absent; the relay-published envelope shape must match `relayEnvelope` (verify against `nats/relay.go` `envelope()`).
- Subscribe-first ordering is load-bearing: the two `Subscribe` calls MUST precede the catch-up DB read, or a fast event between read and subscribe is lost. Do not reorder.
- `create_and_stream` writes headers (200) then streams; a create error must be returned as a normal HTTP error BEFORE any SSE header is written (validate/create first, stream second).

## Notes for the implementer

- Thin passthrough only: emit the real event types/payloads; do not synthesize LangGraph stream modes.
- `stream_thread` uses `closeOnTerminal=false` (stays open until disconnect); the per-run/create variants use `true`.
- Do not push/PR/merge without explicit approval.
