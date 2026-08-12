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

// RegisterThreads mounts the threads endpoints on g (the /api/v1 group).
func (s *Server) RegisterThreads(g *echo.Group) {
	g.POST("/threads", s.ThreadsCreate)
	g.POST("/threads/search", s.ThreadsSearch)
	g.POST("/threads/count", s.ThreadsCount)
	g.GET("/threads/:id", s.ThreadsGet)
	g.PUT("/threads/:id", s.ThreadsUpdate)
	g.DELETE("/threads/:id", s.ThreadsDelete)
	g.GET("/threads/:id/state", s.ThreadsGetState)
	g.GET("/threads/:id/state/:checkpoint_id", s.ThreadsGetCheckpointState)
	g.POST("/threads/:id/state/checkpoint", s.ThreadsCreateCheckpoint)
	g.GET("/threads/:id/history", s.ThreadsGetHistory)
	g.POST("/threads/:id/copy", s.ThreadsCopy)
}

// ThreadsCreate — POST /threads  (kind: write)
//   - INSERT events: event_type='thread.created', payload={metadata}
//   - INSERT outbox (same event_id, same TX)
//   - pg_notify('outbox_new',”)
//   - INSERT threads projection (id, metadata)
func (s *Server) ThreadsCreate(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	var req ThreadCreate
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	aggID := uuid.New()
	payload := mustJSON(req)
	events := []Event{
		{AggregateType: "Thread", AggregateID: aggID, EventType: "thread.created", Payload: payload},
	}
	var row threadRow
	if err := s.writeTx(ctx, s.Tenant, events, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `INSERT INTO threads (id, metadata)
VALUES ($1, $2)
RETURNING id, status, values, config, metadata, created_at, updated_at
`, aggID, mustJSON(req.Metadata))
		if err != nil {
			return err
		}
		row, err = pgx.CollectOneRow(rows, pgx.RowToStructByName[threadRow])
		return err
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusCreated, row.toAPI())
}

// ThreadsSearch — POST /threads/search  (kind: read)
//   - SELECT * FROM threads WHERE metadata @> :filter ORDER BY created_at DESC LIMIT :limit
func (s *Server) ThreadsSearch(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	var req ThreadSearchRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	rows, err := s.Tenant.Query(ctx, `SELECT id, status, values, config, metadata, created_at, updated_at
FROM threads
WHERE ($1::jsonb IS NULL OR metadata @> $1::jsonb)
ORDER BY created_at DESC
LIMIT $2 OFFSET $3
`, jsonbOrNil(req.Metadata), intOr(req.Limit, 20), intOr(req.Offset, 0))
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	list, err := pgx.CollectRows(rows, pgx.RowToStructByName[threadRow])
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	out := make([]Thread, len(list))
	for i := range list {
		out[i] = list[i].toAPI()
	}
	return c.JSON(http.StatusOK, out)
}

// ThreadsCount — POST /threads/count  (kind: read)
//   - SELECT count(*) FROM threads WHERE metadata @> :filter
func (s *Server) ThreadsCount(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	var req ThreadCountRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	var n int
	if err := s.Tenant.QueryRow(ctx, `SELECT count(*) FROM threads
WHERE ($1::jsonb IS NULL OR metadata @> $1::jsonb)
`, jsonbOrNil(req.Metadata)).Scan(&n); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, n)
}

// ThreadsGet — GET /threads/{id}  (kind: read)
//   - SELECT * FROM threads WHERE id = :id
func (s *Server) ThreadsGet(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	pathID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	rows, err := s.Tenant.Query(ctx, `SELECT id, status, values, config, metadata, created_at, updated_at
FROM threads WHERE id = $1
`, pathID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	row, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[threadRow])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, row.toAPI())
}

// ThreadsUpdate — PUT /threads/{id}  (kind: write)
//   - INSERT events: event_type='thread.updated', payload={metadata}
//   - INSERT outbox (same event_id, same TX)
//   - pg_notify('outbox_new',”)
//   - UPDATE threads SET metadata = :metadata WHERE id = :id
func (s *Server) ThreadsUpdate(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	pathID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	var req ThreadPatch
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	payload := mustJSON(req)
	events := []Event{
		{AggregateType: "Thread", AggregateID: pathID, EventType: "thread.updated", Payload: payload},
	}
	var row threadRow
	if err := s.writeTx(ctx, s.Tenant, events, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `UPDATE threads SET
  metadata = COALESCE($2, metadata)
WHERE id = $1
RETURNING id, status, values, config, metadata, created_at, updated_at
`, pathID, jsonbOrNil(req.Metadata))
		if err != nil {
			return err
		}
		row, err = pgx.CollectOneRow(rows, pgx.RowToStructByName[threadRow])
		return err
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, row.toAPI())
}

// ThreadsDelete — DELETE /threads/{id}  (kind: delete)
//   - DELETE threads WHERE id = :id (CASCADE → messages, runs)  # hard delete, no event sourcing
func (s *Server) ThreadsDelete(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	pathID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	ct, err := s.Tenant.Exec(ctx, `DELETE FROM threads WHERE id = $1`, pathID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if ct.RowsAffected() == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}
	return c.NoContent(http.StatusOK)
}

// ThreadsGetState — GET /threads/{id}/state  (kind: read) — hand-written in threads.go

// ThreadsGetCheckpointState — GET /threads/{id}/state/{checkpoint_id}  (kind: read) — hand-written in threads.go

// ThreadsCreateCheckpoint — POST /threads/{id}/state/checkpoint  (kind: read) — hand-written in threads.go

// ThreadsGetHistory — GET /threads/{id}/history  (kind: read) — hand-written in threads.go

// ThreadsCopy — POST /threads/{id}/copy  (kind: write) — hand-written in threads.go
