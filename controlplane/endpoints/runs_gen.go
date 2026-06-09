// Code generated from controlplane/gen/endpoints.yaml — DO NOT EDIT.
// Source of truth: spec/models/d2/endpoint-queries.d2 (via endpoints.yaml).
// Regenerate: go run ./controlplane/gen
package endpoints

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

// RegisterRuns mounts the runs endpoints on g (the /api/v1 group).
func (s *Server) RegisterRuns(g *echo.Group) {
	g.POST("/threads/:id/runs", s.RunsCreateOnThread)
	g.POST("/runs", s.RunsCreateStateless)
	g.POST("/runs/batch", s.RunsBatchCreate)
	g.GET("/threads/:id/runs/:rid", s.RunsGet)
	g.POST("/threads/:id/runs/:rid/cancel", s.RunsCancel)
	g.POST("/threads/:id/runs/:rid/join", s.RunsJoin)
	g.GET("/threads/:id/runs/:rid/stream", s.RunsStreamPerRun)
	g.GET("/threads/:id/stream", s.RunsStreamThread)
	g.POST("/threads/:id/runs/stream", s.RunsCreateAndStream)
	g.POST("/runs/stream", s.RunsStatelessStream)
	g.POST("/runs/wait", s.RunsStatelessWait)
	g.POST("/runs/cancel", s.RunsCancelStateless)
	g.POST("/threads/:id/runs/:rid/resume", s.RunsResume)
}

// RunsCreateOnThread — POST /threads/{id}/runs  (kind: write)
//   - INSERT event_streams (stream_id, aggregate_type='Run', aggregate_id)
//   - INSERT events: event_type='run.created', payload={thread_id, assistant_id, input}
//   - INSERT outbox (same event_id, same TX)
//   - pg_notify('outbox_new',”)
//   - INSERT runs projection (status='queued', thread_id, assistant_id, input)
func (s *Server) RunsCreateOnThread(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	var req map[string]any // TODO: bind OpenAPI type (RunsCreateOnThread request schema)
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	aggID := uuid.New() // TODO: new id for create; parse from path param for update/cancel/etc.
	events := []Event{
		{AggregateType: "Run", AggregateID: aggID, EventType: "run.created"},
	}
	if err := s.writeTx(ctx, s.Tenant, events, func(tx pgx.Tx) error {
		// TODO projection write:
		//   INSERT event_streams (stream_id, aggregate_type='Run', aggregate_id)
		//   INSERT events: event_type='run.created', payload={thread_id, assistant_id, input}
		//   INSERT outbox (same event_id, same TX)
		//   pg_notify('outbox_new','')
		//   INSERT runs projection (status='queued', thread_id, assistant_id, input)
		return nil
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{}) // TODO: return OpenAPI response type
}

// RunsCreateStateless — POST /runs  (kind: write)
//   - INSERT event_streams (stream_id, aggregate_type='Run', aggregate_id)
//   - INSERT events: event_type='run.created', payload={assistant_id, input}
//   - INSERT outbox (same event_id, same TX)
//   - pg_notify('outbox_new',”)
//   - INSERT runs projection (status='queued', no thread_id)
func (s *Server) RunsCreateStateless(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	var req map[string]any // TODO: bind OpenAPI type (RunsCreateStateless request schema)
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	aggID := uuid.New() // TODO: new id for create; parse from path param for update/cancel/etc.
	events := []Event{
		{AggregateType: "Run", AggregateID: aggID, EventType: "run.created"},
	}
	if err := s.writeTx(ctx, s.Tenant, events, func(tx pgx.Tx) error {
		// TODO projection write:
		//   INSERT event_streams (stream_id, aggregate_type='Run', aggregate_id)
		//   INSERT events: event_type='run.created', payload={assistant_id, input}
		//   INSERT outbox (same event_id, same TX)
		//   pg_notify('outbox_new','')
		//   INSERT runs projection (status='queued', no thread_id)
		return nil
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{}) // TODO: return OpenAPI response type
}

// RunsBatchCreate — POST /runs/batch  (kind: write)
//   - FOR EACH run in batch: event_streams + events + outbox + projection
//   - pg_notify('outbox_new',”) once at end of TX
func (s *Server) RunsBatchCreate(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	var req map[string]any // TODO: bind OpenAPI type (RunsBatchCreate request schema)
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	aggID := uuid.New() // TODO: new id for create; parse from path param for update/cancel/etc.
	events := []Event{
		{AggregateType: "Run", AggregateID: aggID, EventType: "run.created"},
	}
	if err := s.writeTx(ctx, s.Tenant, events, func(tx pgx.Tx) error {
		// TODO projection write:
		//   FOR EACH run in batch: event_streams + events + outbox + projection
		//   pg_notify('outbox_new','') once at end of TX
		return nil
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{}) // TODO: return OpenAPI response type
}

// RunsGet — GET /threads/{id}/runs/{rid}  (kind: read)
//   - SELECT * FROM runs WHERE id = :rid AND thread_id = :id
func (s *Server) RunsGet(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	// TODO read query (no side effects):
	//   SELECT * FROM runs WHERE id = :rid AND thread_id = :id
	return c.JSON(http.StatusOK, map[string]any{})
}

// RunsCancel — POST /threads/{id}/runs/{rid}/cancel  (kind: write)
//   - SELECT version FROM runs WHERE id = :rid (optimistic concurrency)
//   - INSERT events: event_type='run.cancelled', payload={reason}
//   - INSERT outbox (same event_id, same TX)
//   - pg_notify('outbox_new',”)
//   - UPDATE runs SET status='cancelled', version=version+1 WHERE id=:rid AND version=:expected
func (s *Server) RunsCancel(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	var req map[string]any // TODO: bind OpenAPI type (RunsCancel request schema)
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	aggID := uuid.New() // TODO: new id for create; parse from path param for update/cancel/etc.
	events := []Event{
		{AggregateType: "Run", AggregateID: aggID, EventType: "run.cancelled"},
	}
	if err := s.writeTx(ctx, s.Tenant, events, func(tx pgx.Tx) error {
		// TODO projection write:
		//   SELECT version FROM runs WHERE id = :rid (optimistic concurrency)
		//   INSERT events: event_type='run.cancelled', payload={reason}
		//   INSERT outbox (same event_id, same TX)
		//   pg_notify('outbox_new','')
		//   UPDATE runs SET status='cancelled', version=version+1 WHERE id=:rid AND version=:expected
		return nil
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{}) // TODO: return OpenAPI response type
}

// RunsJoin — POST /threads/{id}/runs/{rid}/join  (kind: wait)
//   - SELECT status FROM runs WHERE id = :rid
//   - IF not terminal: subscribe NATS run.completed/run.failed for run_id, block
func (s *Server) RunsJoin(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	// WAIT: create/read then block on a terminal NATS event. Bespoke — fill in.
	//   SELECT status FROM runs WHERE id = :rid
	//   IF not terminal: subscribe NATS run.completed/run.failed for run_id, block
	return echo.NewHTTPError(http.StatusNotImplemented, "wait handler not implemented")
}

// RunsStreamPerRun — GET /threads/{id}/runs/{rid}/stream  (kind: sse)
//   - SELECT status FROM runs WHERE id = :rid (validate exists)
//   - Subscribe NATS JetStream consumer filtered to run_id subjects
//   - Loop: receive → SSE data frame → flush
//   - Cleanup: unsubscribe on client disconnect
func (s *Server) RunsStreamPerRun(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	// SSE: subscribe to NATS, stream frames to client. Bespoke — fill in.
	//   SELECT status FROM runs WHERE id = :rid (validate exists)
	//   Subscribe NATS JetStream consumer filtered to run_id subjects
	//   Loop: receive → SSE data frame → flush
	//   Cleanup: unsubscribe on client disconnect
	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
	return echo.NewHTTPError(http.StatusNotImplemented, "sse handler not implemented")
}

// RunsStreamThread — GET /threads/{id}/stream  (kind: sse)
//   - SELECT id FROM runs WHERE thread_id = :id AND status IN ('queued','in_progress')
//   - Subscribe NATS for all active runs on thread
//   - Loop: receive → SSE → flush
func (s *Server) RunsStreamThread(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	// SSE: subscribe to NATS, stream frames to client. Bespoke — fill in.
	//   SELECT id FROM runs WHERE thread_id = :id AND status IN ('queued','in_progress')
	//   Subscribe NATS for all active runs on thread
	//   Loop: receive → SSE → flush
	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
	return echo.NewHTTPError(http.StatusNotImplemented, "sse handler not implemented")
}

// RunsCreateAndStream — POST /threads/{id}/runs/stream  (kind: sse)
//   - CREATE RUN (same as POST /threads/{id}/runs)
//   - Subscribe NATS immediately to new run's subjects
//   - Loop: SSE stream until terminal
func (s *Server) RunsCreateAndStream(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	// SSE: subscribe to NATS, stream frames to client. Bespoke — fill in.
	//   CREATE RUN (same as POST /threads/{id}/runs)
	//   Subscribe NATS immediately to new run's subjects
	//   Loop: SSE stream until terminal
	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
	return echo.NewHTTPError(http.StatusNotImplemented, "sse handler not implemented")
}

// RunsStatelessStream — POST /runs/stream  (kind: sse)
//   - CREATE RUN (same as POST /runs)
//   - Subscribe NATS immediately to new run's subjects
//   - Loop: SSE stream until terminal
func (s *Server) RunsStatelessStream(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	// SSE: subscribe to NATS, stream frames to client. Bespoke — fill in.
	//   CREATE RUN (same as POST /runs)
	//   Subscribe NATS immediately to new run's subjects
	//   Loop: SSE stream until terminal
	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
	return echo.NewHTTPError(http.StatusNotImplemented, "sse handler not implemented")
}

// RunsStatelessWait — POST /runs/wait  (kind: wait)
//   - CREATE RUN (same as POST /runs)
//   - Subscribe NATS, wait for run.completed or run.failed
//   - Return final run state as JSON
func (s *Server) RunsStatelessWait(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	// WAIT: create/read then block on a terminal NATS event. Bespoke — fill in.
	//   CREATE RUN (same as POST /runs)
	//   Subscribe NATS, wait for run.completed or run.failed
	//   Return final run state as JSON
	return echo.NewHTTPError(http.StatusNotImplemented, "wait handler not implemented")
}

// RunsCancelStateless — POST /runs/cancel  (kind: write)
//   - same as POST /threads/{id}/runs/{rid}/cancel
func (s *Server) RunsCancelStateless(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	var req map[string]any // TODO: bind OpenAPI type (RunsCancelStateless request schema)
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	aggID := uuid.New() // TODO: new id for create; parse from path param for update/cancel/etc.
	events := []Event{
		{AggregateType: "Run", AggregateID: aggID, EventType: "run.cancelled"},
	}
	if err := s.writeTx(ctx, s.Tenant, events, func(tx pgx.Tx) error {
		// TODO projection write:
		//   same as POST /threads/{id}/runs/{rid}/cancel
		return nil
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{}) // TODO: return OpenAPI response type
}

// RunsResume — POST /threads/{id}/runs/{rid}/resume  (kind: write)
//   - SELECT runs WHERE id=:rid AND status='requires_action' (validate)
//   - SELECT interrupts WHERE run_id=:rid AND resolved=false
//   - UPDATE interrupts SET resolved=true, resolved_at=now()
//   - IF command.update: merge state updates into run input
//   - INSERT events: event_type='run.resumed', payload={interrupt_id, tool_outputs, command}
//   - INSERT outbox (same event_id, same TX)
//   - pg_notify('outbox_new',”)
//   - UPDATE runs SET status='in_progress', input=merged_input, version=version+1
//   - Re-dispatch: ExecuteRun() — worker claims with checkpoint_id
func (s *Server) RunsResume(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	var req map[string]any // TODO: bind OpenAPI type (RunsResume request schema)
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	aggID := uuid.New() // TODO: new id for create; parse from path param for update/cancel/etc.
	events := []Event{
		{AggregateType: "Run", AggregateID: aggID, EventType: "run.resumed"},
	}
	if err := s.writeTx(ctx, s.Tenant, events, func(tx pgx.Tx) error {
		// TODO projection write:
		//   SELECT runs WHERE id=:rid AND status='requires_action' (validate)
		//   SELECT interrupts WHERE run_id=:rid AND resolved=false
		//   UPDATE interrupts SET resolved=true, resolved_at=now()
		//   IF command.update: merge state updates into run input
		//   INSERT events: event_type='run.resumed', payload={interrupt_id, tool_outputs, command}
		//   INSERT outbox (same event_id, same TX)
		//   pg_notify('outbox_new','')
		//   UPDATE runs SET status='in_progress', input=merged_input, version=version+1
		//   Re-dispatch: ExecuteRun() — worker claims with checkpoint_id
		return nil
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{}) // TODO: return OpenAPI response type
}
