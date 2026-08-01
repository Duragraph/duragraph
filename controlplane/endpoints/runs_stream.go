// Hand-written SSE + wait handlers for the runs group (routes generated into
// runs_gen.go via custom: true). Lossless: subscribe-first, replay persisted
// events (catch-up), then stream live NATS events deduped by event_id, closing on
// the run's terminal event or client disconnect. Thin passthrough frames.
//
// Task 1 (this file, initial cut) implements the shared plumbing (streamRun,
// writeSSEFrame, isTerminalEvent, relayEnvelope) and RunsStreamPerRun in full.
// RunsJoin, RunsStreamThread, RunsCreateAndStream, RunsStatelessStream, and
// RunsStatelessWait are stubbed here with the same NotImplemented behavior the
// generator previously produced — Tasks 2 and 3 replace the stubs with real
// bodies built on streamRun (see
// controlplane/docs/superpowers/plans/2026-07-31-runs-streaming-wait.md).
package endpoints

import (
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

// --- Stubs below: Task 1 marks these endpoints custom (so Task 2/3 can build
// on streamRun without another generator/regen round), but only implements
// RunsStreamPerRun. The stubs reproduce the exact NotImplemented behavior the
// generator previously emitted for these (kind: sse / kind: wait, no impl),
// so this is a no-behavior-change placeholder pending Tasks 2 and 3.

// RunsJoin — POST /threads/{id}/runs/{rid}/join  (kind: wait)
//   - SELECT status FROM runs WHERE id = :rid
//   - IF not terminal: subscribe NATS run.completed/run.failed for run_id, block
//
// TODO(Task 3): implement via waitForRun (see the streaming-wait plan).
func (s *Server) RunsJoin(c echo.Context) error {
	return echo.NewHTTPError(http.StatusNotImplemented, "wait handler not implemented")
}

// RunsStreamThread — GET /threads/{id}/stream  (kind: sse)
//   - SELECT id FROM runs WHERE thread_id = :id AND status IN ('queued','in_progress')
//   - Subscribe NATS for all active runs on thread
//   - Loop: receive → SSE → flush
//
// TODO(Task 2): implement via streamRun(c, ids, false).
func (s *Server) RunsStreamThread(c echo.Context) error {
	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
	return echo.NewHTTPError(http.StatusNotImplemented, "sse handler not implemented")
}

// RunsCreateAndStream — POST /threads/{id}/runs/stream  (kind: sse)
//   - CREATE RUN (same as POST /threads/{id}/runs)
//   - Subscribe NATS immediately to new run's subjects
//   - Loop: SSE stream until terminal
//
// TODO(Task 2): implement via createRun + streamRun(c, {rid}, true).
func (s *Server) RunsCreateAndStream(c echo.Context) error {
	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
	return echo.NewHTTPError(http.StatusNotImplemented, "sse handler not implemented")
}

// RunsStatelessStream — POST /runs/stream  (kind: sse)
//   - CREATE RUN (same as POST /runs)
//   - Subscribe NATS immediately to new run's subjects
//   - Loop: SSE stream until terminal
//
// TODO(Task 2): implement via createRun(nil, ...) + streamRun(c, {rid}, true).
func (s *Server) RunsStatelessStream(c echo.Context) error {
	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
	return echo.NewHTTPError(http.StatusNotImplemented, "sse handler not implemented")
}

// RunsStatelessWait — POST /runs/wait  (kind: wait)
//   - CREATE RUN (same as POST /runs)
//   - Subscribe NATS, wait for run.completed or run.failed
//   - Return final run state as JSON
//
// TODO(Task 3): implement via createRun(nil, ...) + waitForRun.
func (s *Server) RunsStatelessWait(c echo.Context) error {
	return echo.NewHTTPError(http.StatusNotImplemented, "wait handler not implemented")
}
