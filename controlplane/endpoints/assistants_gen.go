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

// RegisterAssistants mounts the assistants endpoints on g (the /api/v1 group).
func (s *Server) RegisterAssistants(g *echo.Group) {
	g.POST("/assistants", s.AssistantsCreate)
	g.POST("/assistants/search", s.AssistantsSearch)
	g.POST("/assistants/count", s.AssistantsCount)
	g.GET("/assistants/:id", s.AssistantsGet)
	g.PUT("/assistants/:id", s.AssistantsUpdate)
	g.DELETE("/assistants/:id", s.AssistantsDelete)
	g.GET("/assistants/:id/graph", s.AssistantsGetGraph)
	g.GET("/assistants/:id/versions", s.AssistantsGetVersions)
	g.POST("/assistants/:id/latest", s.AssistantsSetLatest)
}

// AssistantsCreate — POST /assistants  (kind: write)
//   - INSERT events: event_type='assistant.created', payload={name, graph_id, config, tools}
//   - INSERT outbox (same event_id, same TX)
//   - pg_notify('outbox_new',”)
//   - INSERT assistants projection (id, graph_id, name, model, tools, config)
func (s *Server) AssistantsCreate(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	var req AssistantCreate
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	aggID := uuid.New()
	payload := mustJSON(req)
	events := []Event{
		{AggregateType: "Assistant", AggregateID: aggID, EventType: "assistant.created", Payload: payload},
	}
	var row assistantRow
	if err := s.writeTx(ctx, s.Tenant, events, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `INSERT INTO assistants (id, graph_id, name, description, config, metadata)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, graph_id, name, description, model, instructions, tools, config, metadata, created_at, updated_at
`, aggID, req.GraphId, deref(req.Name), req.Description, mustJSON(req.Config), mustJSON(req.Metadata))
		if err != nil {
			return err
		}
		row, err = pgx.CollectOneRow(rows, pgx.RowToStructByName[assistantRow])
		return err
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusCreated, row.toAPI())
}

// AssistantsSearch — POST /assistants/search  (kind: read)
//   - SELECT * FROM assistants WHERE metadata @> :filter ORDER BY created_at DESC LIMIT :limit OFFSET :offset
func (s *Server) AssistantsSearch(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	// TODO read query (no side effects):
	//   SELECT * FROM assistants WHERE metadata @> :filter ORDER BY created_at DESC LIMIT :limit OFFSET :offset
	return c.JSON(http.StatusOK, map[string]any{})
}

// AssistantsCount — POST /assistants/count  (kind: read)
//   - SELECT count(*) FROM assistants WHERE metadata @> :filter
func (s *Server) AssistantsCount(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	// TODO read query (no side effects):
	//   SELECT count(*) FROM assistants WHERE metadata @> :filter
	return c.JSON(http.StatusOK, map[string]any{})
}

// AssistantsGet — GET /assistants/{id}  (kind: read)
//   - SELECT * FROM assistants WHERE id = :id
func (s *Server) AssistantsGet(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	pathID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	rows, err := s.Tenant.Query(ctx, `SELECT id, graph_id, name, description, model, instructions, tools, config, metadata, created_at, updated_at
FROM assistants WHERE id = $1
`, pathID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	row, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[assistantRow])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, row.toAPI())
}

// AssistantsUpdate — PUT /assistants/{id}  (kind: write)
//   - INSERT events: event_type='assistant.updated', payload={changed fields}
//   - INSERT outbox (same event_id, same TX)
//   - pg_notify('outbox_new',”)
//   - UPDATE assistants SET name=:name, model=:model, ... WHERE id=:id
func (s *Server) AssistantsUpdate(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	var req map[string]any // TODO: bind OpenAPI type (AssistantsUpdate request schema)
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	aggID := uuid.New() // TODO: new id for create; parse from path param for update/cancel/etc.
	events := []Event{
		{AggregateType: "Assistant", AggregateID: aggID, EventType: "assistant.updated"},
	}
	if err := s.writeTx(ctx, s.Tenant, events, func(tx pgx.Tx) error {
		// TODO projection write:
		//   INSERT events: event_type='assistant.updated', payload={changed fields}
		//   INSERT outbox (same event_id, same TX)
		//   pg_notify('outbox_new','')
		//   UPDATE assistants SET name=:name, model=:model, ... WHERE id=:id
		return nil
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{}) // TODO: return OpenAPI response type
}

// AssistantsDelete — DELETE /assistants/{id}  (kind: write)
//   - INSERT events: event_type='assistant.deleted'
//   - INSERT outbox (same event_id, same TX)
//   - pg_notify('outbox_new',”)
//   - DELETE assistants WHERE id = :id (CASCADE → graphs)
func (s *Server) AssistantsDelete(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	var req map[string]any // TODO: bind OpenAPI type (AssistantsDelete request schema)
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	aggID := uuid.New() // TODO: new id for create; parse from path param for update/cancel/etc.
	events := []Event{
		{AggregateType: "Assistant", AggregateID: aggID, EventType: "assistant.deleted"},
	}
	if err := s.writeTx(ctx, s.Tenant, events, func(tx pgx.Tx) error {
		// TODO projection write:
		//   INSERT events: event_type='assistant.deleted'
		//   INSERT outbox (same event_id, same TX)
		//   pg_notify('outbox_new','')
		//   DELETE assistants WHERE id = :id (CASCADE → graphs)
		return nil
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{}) // TODO: return OpenAPI response type
}

// AssistantsGetGraph — GET /assistants/{id}/graph  (kind: read)
//   - SELECT * FROM graphs WHERE assistant_id = :id AND version = (SELECT max version)
func (s *Server) AssistantsGetGraph(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	// TODO read query (no side effects):
	//   SELECT * FROM graphs WHERE assistant_id = :id AND version = (SELECT max version)
	return c.JSON(http.StatusOK, map[string]any{})
}

// AssistantsGetVersions — GET /assistants/{id}/versions  (kind: read)
//   - SELECT * FROM graphs WHERE assistant_id = :id ORDER BY version DESC
func (s *Server) AssistantsGetVersions(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	// TODO read query (no side effects):
	//   SELECT * FROM graphs WHERE assistant_id = :id ORDER BY version DESC
	return c.JSON(http.StatusOK, map[string]any{})
}

// AssistantsSetLatest — POST /assistants/{id}/latest  (kind: write)
//   - INSERT events: event_type='graph.updated', payload={graph_id, version}
//   - INSERT outbox (same event_id, same TX)
//   - pg_notify('outbox_new',”)
//   - UPDATE graphs SET assistant_id=:id WHERE name=:graph_id AND version=:v
func (s *Server) AssistantsSetLatest(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	var req map[string]any // TODO: bind OpenAPI type (AssistantsSetLatest request schema)
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	aggID := uuid.New() // TODO: new id for create; parse from path param for update/cancel/etc.
	events := []Event{
		{AggregateType: "Graph", AggregateID: aggID, EventType: "graph.updated"},
	}
	if err := s.writeTx(ctx, s.Tenant, events, func(tx pgx.Tx) error {
		// TODO projection write:
		//   INSERT events: event_type='graph.updated', payload={graph_id, version}
		//   INSERT outbox (same event_id, same TX)
		//   pg_notify('outbox_new','')
		//   UPDATE graphs SET assistant_id=:id WHERE name=:graph_id AND version=:v
		return nil
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{}) // TODO: return OpenAPI response type
}
