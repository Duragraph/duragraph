// Code generated from controlplane/gen/endpoints.yaml — DO NOT EDIT.
// Source of truth: spec/models/d2/endpoint-queries.d2 (via endpoints.yaml).
// Regenerate: go run ./controlplane/gen
package endpoints

import (
	"errors"
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

// RunsCreateOnThread — POST /threads/{id}/runs  (kind: write) — hand-written in runs.go

// RunsCreateStateless — POST /runs  (kind: write) — hand-written in runs.go

// RunsBatchCreate — POST /runs/batch  (kind: write) — hand-written in runs.go

// RunsGet — GET /threads/{id}/runs/{rid}  (kind: read)
//   - SELECT * FROM runs WHERE id = :rid AND thread_id = :id
func (s *Server) RunsGet(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	pathID, err := uuid.Parse(c.Param("rid"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid rid")
	}
	pathID2, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	rows, err := s.Tenant.Query(ctx, `SELECT id, thread_id, assistant_id, status, input, output, error, metadata, kwargs, multitask_strategy, version, lease_epoch, worker_id, priority, graph_id, created_at, started_at, completed_at, updated_at
FROM runs WHERE id = $1 AND thread_id = $2
`, pathID, pathID2)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	row, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[runRow])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, row.toAPI())
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
	pathID, err := uuid.Parse(c.Param("rid"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid rid")
	}
	var payload []byte
	events := []Event{
		{AggregateType: "Run", AggregateID: pathID, EventType: "run.cancelled", Payload: payload},
	}
	var row runRow
	if err := s.writeTx(ctx, s.Tenant, events, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `UPDATE runs SET status='cancelled', version=version+1, updated_at=now()
WHERE id = $1 AND status IN ('queued', 'in_progress', 'requires_action')
RETURNING id, thread_id, assistant_id, status, input, output, error, metadata, kwargs, multitask_strategy, version, lease_epoch, worker_id, priority, graph_id, created_at, started_at, completed_at, updated_at
`, pathID)
		if err != nil {
			return err
		}
		row, err = pgx.CollectOneRow(rows, pgx.RowToStructByName[runRow])
		return err
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, row.toAPI())
}

// RunsJoin — POST /threads/{id}/runs/{rid}/join  (kind: wait) — hand-written in runs.go

// RunsStreamPerRun — GET /threads/{id}/runs/{rid}/stream  (kind: sse) — hand-written in runs.go

// RunsStreamThread — GET /threads/{id}/stream  (kind: sse) — hand-written in runs.go

// RunsCreateAndStream — POST /threads/{id}/runs/stream  (kind: sse) — hand-written in runs.go

// RunsStatelessStream — POST /runs/stream  (kind: sse) — hand-written in runs.go

// RunsStatelessWait — POST /runs/wait  (kind: wait) — hand-written in runs.go

// RunsCancelStateless — POST /runs/cancel  (kind: write) — hand-written in runs.go

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
