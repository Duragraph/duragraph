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
	var req map[string]any // TODO: bind OpenAPI type (ThreadsCreate request schema)
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	aggID := uuid.New() // TODO: new id for create; parse from path param for update/cancel/etc.
	events := []Event{
		{AggregateType: "Thread", AggregateID: aggID, EventType: "thread.created"},
	}
	if err := s.writeTx(ctx, s.Tenant, events, func(tx pgx.Tx) error {
		// TODO projection write:
		//   INSERT events: event_type='thread.created', payload={metadata}
		//   INSERT outbox (same event_id, same TX)
		//   pg_notify('outbox_new','')
		//   INSERT threads projection (id, metadata)
		return nil
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{}) // TODO: return OpenAPI response type
}

// ThreadsSearch — POST /threads/search  (kind: read)
//   - SELECT * FROM threads WHERE metadata @> :filter ORDER BY created_at DESC LIMIT :limit
func (s *Server) ThreadsSearch(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	// TODO read query (no side effects):
	//   SELECT * FROM threads WHERE metadata @> :filter ORDER BY created_at DESC LIMIT :limit
	return c.JSON(http.StatusOK, map[string]any{})
}

// ThreadsCount — POST /threads/count  (kind: read)
//   - SELECT count(*) FROM threads WHERE metadata @> :filter
func (s *Server) ThreadsCount(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	// TODO read query (no side effects):
	//   SELECT count(*) FROM threads WHERE metadata @> :filter
	return c.JSON(http.StatusOK, map[string]any{})
}

// ThreadsGet — GET /threads/{id}  (kind: read)
//   - SELECT * FROM threads WHERE id = :id
func (s *Server) ThreadsGet(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	// TODO read query (no side effects):
	//   SELECT * FROM threads WHERE id = :id
	return c.JSON(http.StatusOK, map[string]any{})
}

// ThreadsUpdate — PUT /threads/{id}  (kind: write)
//   - INSERT events: event_type='thread.updated', payload={metadata}
//   - INSERT outbox (same event_id, same TX)
//   - pg_notify('outbox_new',”)
//   - UPDATE threads SET metadata = :metadata WHERE id = :id
func (s *Server) ThreadsUpdate(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	var req map[string]any // TODO: bind OpenAPI type (ThreadsUpdate request schema)
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	aggID := uuid.New() // TODO: new id for create; parse from path param for update/cancel/etc.
	events := []Event{
		{AggregateType: "Thread", AggregateID: aggID, EventType: "thread.updated"},
	}
	if err := s.writeTx(ctx, s.Tenant, events, func(tx pgx.Tx) error {
		// TODO projection write:
		//   INSERT events: event_type='thread.updated', payload={metadata}
		//   INSERT outbox (same event_id, same TX)
		//   pg_notify('outbox_new','')
		//   UPDATE threads SET metadata = :metadata WHERE id = :id
		return nil
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{}) // TODO: return OpenAPI response type
}

// ThreadsDelete — DELETE /threads/{id}  (kind: delete)
//   - DELETE threads WHERE id = :id (CASCADE → messages, runs)  # hard delete, no event sourcing
func (s *Server) ThreadsDelete(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	// TODO hard delete:
	//   DELETE threads WHERE id = :id (CASCADE → messages, runs)  # hard delete, no event sourcing
	return c.NoContent(http.StatusNoContent)
}

// ThreadsGetState — GET /threads/{id}/state  (kind: read)
//   - SELECT * FROM messages WHERE thread_id = :id ORDER BY created_at
//   - SELECT * FROM runs WHERE thread_id = :id AND status='completed' ORDER BY completed_at DESC LIMIT 1
func (s *Server) ThreadsGetState(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	// TODO read query (no side effects):
	//   SELECT * FROM messages WHERE thread_id = :id ORDER BY created_at
	//   SELECT * FROM runs WHERE thread_id = :id AND status='completed' ORDER BY completed_at DESC LIMIT 1
	return c.JSON(http.StatusOK, map[string]any{})
}

// ThreadsGetCheckpointState — GET /threads/{id}/state/{checkpoint_id}  (kind: read)
//   - SELECT * FROM snapshots WHERE stream_id = :checkpoint_id
func (s *Server) ThreadsGetCheckpointState(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	// TODO read query (no side effects):
	//   SELECT * FROM snapshots WHERE stream_id = :checkpoint_id
	return c.JSON(http.StatusOK, map[string]any{})
}

// ThreadsCreateCheckpoint — POST /threads/{id}/state/checkpoint  (kind: write)
//   - INSERT snapshots (stream_id, aggregate_type, aggregate_id, version, state)  # infra, not domain event
func (s *Server) ThreadsCreateCheckpoint(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	var req map[string]any // TODO: bind OpenAPI type (ThreadsCreateCheckpoint request schema)
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	// projection-only write (not event-sourced — no outbox):
	//   INSERT snapshots (stream_id, aggregate_type, aggregate_id, version, state)  # infra, not domain event
	return c.JSON(http.StatusOK, map[string]any{}) // TODO: return OpenAPI response type
}

// ThreadsGetHistory — GET /threads/{id}/history  (kind: read)
//   - SELECT * FROM events WHERE aggregate_id IN (SELECT id FROM runs WHERE thread_id=:id) ORDER BY occurred_at
func (s *Server) ThreadsGetHistory(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	// TODO read query (no side effects):
	//   SELECT * FROM events WHERE aggregate_id IN (SELECT id FROM runs WHERE thread_id=:id) ORDER BY occurred_at
	return c.JSON(http.StatusOK, map[string]any{})
}

// ThreadsCopy — POST /threads/{id}/copy  (kind: write)
//   - INSERT threads (new id, copy metadata)
//   - INSERT messages (copy all with new thread_id)
//   - INSERT events: event_type='thread.created' for the new thread
//   - INSERT outbox (same TX)
//   - pg_notify('outbox_new',”)
func (s *Server) ThreadsCopy(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	var req map[string]any // TODO: bind OpenAPI type (ThreadsCopy request schema)
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	aggID := uuid.New() // TODO: new id for create; parse from path param for update/cancel/etc.
	events := []Event{
		{AggregateType: "Thread", AggregateID: aggID, EventType: "thread.created"},
	}
	if err := s.writeTx(ctx, s.Tenant, events, func(tx pgx.Tx) error {
		// TODO projection write:
		//   INSERT threads (new id, copy metadata)
		//   INSERT messages (copy all with new thread_id)
		//   INSERT events: event_type='thread.created' for the new thread
		//   INSERT outbox (same TX)
		//   pg_notify('outbox_new','')
		return nil
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{}) // TODO: return OpenAPI response type
}
