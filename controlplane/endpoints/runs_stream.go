// Hand-written SSE + wait handlers for the runs group (routes generated into
// runs_gen.go via custom: true). Lossless: subscribe-first, replay persisted
// events (catch-up), then stream live NATS events deduped by event_id, closing on
// the run's terminal event or client disconnect. Thin passthrough frames.
//
// Task 1 implemented the shared plumbing (streamRun, writeSSEFrame,
// isTerminalEvent, relayEnvelope) and RunsStreamPerRun in full. Task 2 added
// RunsStreamThread, RunsCreateAndStream, RunsStatelessStream, and the shared
// createRun helper, all built on streamRun. Task 3 (this pass) adds
// waitForRun (block-until-terminal, subscribe-first) plus RunsJoin and
// RunsStatelessWait, built on it.
package endpoints

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/duragraph/duragraph/controlplane/nats"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

// seenCap bounds the memory a single streamRun connection's dedup set can use.
// Per-run streams (closeOnTerminal=true) close quickly and never approach
// this; the thread feed (closeOnTerminal=false) can run indefinitely, so
// without a bound its dedup set would grow forever. Evicting the oldest seen
// event_id once the cap is exceeded trades perfect dedup for very old events
// (rare — a duplicate that old would surface as a harmless repeat frame, never
// a missed one) for a fixed memory ceiling.
const seenCap = 4096

// boundedSeen is a fixed-capacity, O(1) set of event_id strings backed by a
// ring buffer: once full, each Add overwrites (and evicts from the map) the
// oldest entry.
type boundedSeen struct {
	set  map[string]bool
	ring []string
	pos  int
	full bool
}

func newBoundedSeen(cap int) *boundedSeen {
	return &boundedSeen{set: make(map[string]bool, cap), ring: make([]string, cap)}
}

func (b *boundedSeen) Has(id string) bool { return b.set[id] }

func (b *boundedSeen) Add(id string) {
	if b.set[id] {
		return
	}
	if b.full {
		delete(b.set, b.ring[b.pos])
	}
	b.ring[b.pos] = id
	b.set[id] = true
	b.pos++
	if b.pos == len(b.ring) {
		b.pos = 0
		b.full = true
	}
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

	seen := newBoundedSeen(seenCap) // event_id → emitted (dedup, bounded)

	// 2. Catch-up: replay persisted events for the watched runs, in order.
	// Ordered by occurred_at first (event_version alone only orders events
	// WITHIN one run's own stream; for a multi-run watch set — e.g. the
	// thread feed, which unions every run on a thread — sorting by version
	// alone interleaves each run's independently-numbered version sequence
	// wrongly, since a low version number on one run has no relationship to
	// a low version number on another. occurred_at gives a single, real
	// chronological order across runs; event_version stays as the tiebreak
	// for same-instant events, so single-run ordering is unchanged.
	ids := make([]uuid.UUID, 0, len(runIDs))
	for id := range runIDs {
		ids = append(ids, id)
	}
	rows, err := s.Tenant.Query(ctx, `
		SELECT event_id::text, event_type, payload FROM events
		WHERE aggregate_id = ANY($1) ORDER BY occurred_at, event_version`, ids)
	if err == nil {
		for rows.Next() {
			var eid, etype string
			var payload []byte
			if rows.Scan(&eid, &etype, &payload) == nil {
				seen.Add(eid)
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
		if err != nil || !runIDs[aid] || seen.Has(env.EventID) {
			continue
		}
		seen.Add(env.EventID)
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

// RunsStreamThread — GET /threads/{id}/stream  (kind: sse)
// Streams every run currently on the thread, unioned. The feed stays open
// until the client disconnects (closeOnTerminal=false): a thread outlives any
// single run, so a terminal event on one run must not end the whole feed. An
// empty run set (a brand-new thread with no runs yet) is valid — nothing to
// catch up on, and the live loop simply drops every envelope until a watched
// aggregate_id shows up. Known limitation: runIDs is a snapshot taken once at
// request time, so a run created on this thread AFTER the stream opens is not
// added to the watch set (its events are silently ignored, not missed-then-
// caught-up) — reconnecting picks it up. Fine for now; a future pass could
// re-query periodically or subscribe to run.created for the thread.
func (s *Server) RunsStreamThread(c echo.Context) error {
	if s.Subscriber == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "streaming requires NATS")
	}
	ctx := c.Request().Context()
	threadID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	ids := map[uuid.UUID]bool{}
	rows, err := s.Tenant.Query(ctx, `SELECT id FROM runs WHERE thread_id = $1`, threadID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	for rows.Next() {
		var id uuid.UUID
		if scanErr := rows.Scan(&id); scanErr == nil {
			ids[id] = true
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return s.streamRun(c, ids, false /*stay open — thread feed*/)
}

// createRun is the shared create path for the two stream-and-create
// endpoints below. It mirrors RunsCreateOnThread/RunsCreateStateless's write
// (event_streams + events + outbox + runs projection, all in one TX via
// writeTx) but is parameterized on threadID so both variants can share it —
// the generated handlers keep their own inline copy so this stays a small,
// additive helper rather than a generator change. Returns the new run's id.
// kwargs carries the run-level LangGraph kwargs (interrupt_before /
// interrupt_after) already validated by the caller via buildRunKwargs; pass
// nil for none and the column default '{}' applies.
func (s *Server) createRun(ctx context.Context, threadID *uuid.UUID, assistantID uuid.UUID, input, metadata, kwargs []byte) (uuid.UUID, error) {
	aggID := uuid.New()
	payload := mustJSON(struct {
		AssistantID uuid.UUID       `json:"assistant_id"`
		Input       json.RawMessage `json:"input,omitempty"`
		Metadata    json.RawMessage `json:"metadata,omitempty"`
	}{AssistantID: assistantID, Input: input, Metadata: metadata})
	events := []Event{
		{AggregateType: "Run", AggregateID: aggID, EventType: "run.created", Payload: payload},
	}
	err := s.writeTx(ctx, s.Tenant, events, func(tx pgx.Tx) error {
		var execErr error
		if threadID != nil {
			_, execErr = tx.Exec(ctx, `INSERT INTO runs (id, thread_id, assistant_id, status, input, metadata, kwargs)
VALUES ($1, $2, $3, 'queued', $4, $5, $6)`, aggID, *threadID, assistantID, input, metadata, jsonOrEmpty(kwargs))
		} else {
			_, execErr = tx.Exec(ctx, `INSERT INTO runs (id, assistant_id, status, input, metadata, kwargs)
VALUES ($1, $2, 'queued', $3, $4, $5)`, aggID, assistantID, input, metadata, jsonOrEmpty(kwargs))
		}
		return execErr
	})
	if err != nil {
		return uuid.UUID{}, err
	}
	return aggID, nil
}

// RunsCreateAndStream — POST /threads/{id}/runs/stream  (kind: sse)
// Creates the run first (a normal HTTP error on a bad request/DB failure,
// with no SSE header written yet), then streams that one run until terminal.
func (s *Server) RunsCreateAndStream(c echo.Context) error {
	if s.Subscriber == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "streaming requires NATS")
	}
	ctx := c.Request().Context()
	threadID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	var req RunCreateStateful
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	assistantID, err := s.resolveAssistantRef(ctx, req.AssistantId)
	if err != nil {
		return assistantRefHTTPError(err)
	}
	checkpoint, err := normalizeCheckpoint(req.Checkpoint, threadID)
	if err != nil {
		return interruptSpecHTTPError(err)
	}
	kwargs, err := buildRunKwargs(req.InterruptBefore, req.InterruptAfter, req.Command, checkpoint)
	if err != nil {
		return interruptSpecHTTPError(err)
	}
	if err := s.verifyCheckpointOwned(ctx, checkpoint, threadID); err != nil {
		return checkpointHTTPError(err)
	}
	rid, err := s.createRun(ctx, &threadID, assistantID, mustJSON(req.Input), mustJSON(req.Metadata), kwargs)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return s.streamRun(c, map[uuid.UUID]bool{rid: true}, true /*closeOnTerminal*/)
}

// RunsStatelessStream — POST /runs/stream  (kind: sse)
// Same as RunsCreateAndStream but for a stateless run (no thread).
func (s *Server) RunsStatelessStream(c echo.Context) error {
	if s.Subscriber == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "streaming requires NATS")
	}
	ctx := c.Request().Context()
	var req RunCreateStateless
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	assistantID, err := s.resolveAssistantRef(ctx, req.AssistantId)
	if err != nil {
		return assistantRefHTTPError(err)
	}
	kwargs, err := buildRunKwargs(req.InterruptBefore, req.InterruptAfter, req.Command, nil)
	if err != nil {
		return interruptSpecHTTPError(err)
	}
	rid, err := s.createRun(ctx, nil, assistantID, mustJSON(req.Input), mustJSON(req.Metadata), kwargs)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return s.streamRun(c, map[uuid.UUID]bool{rid: true}, true /*closeOnTerminal*/)
}

// waitForRun blocks until run rid reaches a terminal status (completed,
// failed, cancelled), then responds 200 with the run's current API
// representation. Subscribe-first (before the DB status read) is load-bearing:
// it guarantees a terminal event fired in the gap between the read and the
// subscribe can never be missed — the same lossless ordering streamRun uses.
//
//   - 503 if NATS is disabled (no Subscriber).
//   - 404 if the run doesn't exist.
//   - If the run is already terminal, returns immediately without waiting.
//   - Otherwise blocks on the live feed until a terminal run.* event for rid
//     arrives, or the request context is canceled (client disconnect/timeout),
//     in which case it returns the run's state as of the cancellation.
func (s *Server) waitForRun(c echo.Context, rid uuid.UUID) error {
	if s.Subscriber == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "wait requires NATS")
	}
	ctx := c.Request().Context()

	// 1. Subscribe FIRST (before the status read) so nothing is missed in the gap.
	runsCh, err := s.Subscriber.Subscribe(ctx, "duragraph.runs.>")
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// 2. Check current status; skip waiting if already terminal.
	var status string
	if err := s.Tenant.QueryRow(ctx, `SELECT status FROM runs WHERE id=$1`, rid).Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "run not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if !isTerminalStatus(status) {
		// 3. Block until a terminal run.* event for rid arrives, or disconnect.
	waitLoop:
		for {
			select {
			case <-ctx.Done():
				break waitLoop
			case msg := <-runsCh:
				if msg == nil { // channel closed (ctx canceled)
					break waitLoop
				}
				var env relayEnvelope
				if json.Unmarshal(msg.Payload, &env) != nil {
					continue
				}
				aid, err := uuid.Parse(env.AggregateID)
				if err != nil || aid != rid || !isTerminalEvent(env.EventType) {
					continue
				}
				break waitLoop
			}
		}
	}

	// 4. Return the run's current state (fresh SELECT — whatever it is at this
	// point, terminal or not, e.g. if the wait ended via client disconnect).
	rows, err := s.Tenant.Query(ctx, `SELECT id, thread_id, assistant_id, status, input, output, error, metadata, kwargs, multitask_strategy, version, lease_epoch, worker_id, priority, graph_id, created_at, started_at, completed_at, updated_at
FROM runs WHERE id = $1`, rid)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	row, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[runRow])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "run not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, row.toAPI())
}

// isTerminalStatus reports whether a DB runs.status value is terminal
// (mirrors isTerminalEvent's event-type check, for the DB-status side).
func isTerminalStatus(status string) bool {
	return status == "completed" || status == "failed" || status == "cancelled"
}

// RunsJoin — POST /threads/{id}/runs/{rid}/join  (kind: wait)
// Blocks until the run reaches a terminal status, then returns it as JSON.
func (s *Server) RunsJoin(c echo.Context) error {
	rid, err := uuid.Parse(c.Param("rid"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid rid")
	}
	return s.waitForRun(c, rid)
}

// RunsStatelessWait — POST /runs/wait  (kind: wait)
// Creates a stateless run, then blocks until it reaches a terminal status and
// returns it as JSON.
func (s *Server) RunsStatelessWait(c echo.Context) error {
	if s.Subscriber == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "wait requires NATS")
	}
	ctx := c.Request().Context()
	var req RunCreateStateless
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	assistantID, err := s.resolveAssistantRef(ctx, req.AssistantId)
	if err != nil {
		return assistantRefHTTPError(err)
	}
	kwargs, err := buildRunKwargs(req.InterruptBefore, req.InterruptAfter, req.Command, nil)
	if err != nil {
		return interruptSpecHTTPError(err)
	}
	rid, err := s.createRun(ctx, nil, assistantID, mustJSON(req.Input), mustJSON(req.Metadata), kwargs)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return s.waitForRun(c, rid)
}
