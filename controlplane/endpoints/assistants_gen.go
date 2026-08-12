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
	g.PATCH("/assistants/:id", s.AssistantsUpdate)
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
		rows, err := tx.Query(ctx, `INSERT INTO assistants (id, graph_id, name, description, config, context, metadata)
VALUES ($1, $2, $3, $4, $5, COALESCE($6::jsonb, '{}'::jsonb), $7)
RETURNING id, graph_id, name, description, model, instructions, tools, config, context, version, metadata, created_at, updated_at
`, aggID, req.GraphId, deref(req.Name), req.Description, mustJSON(req.Config), jsonbOrNil(req.Context), mustJSON(req.Metadata))
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
	var req AssistantSearchRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	rows, err := s.Tenant.Query(ctx, `SELECT id, graph_id, name, description, model, instructions, tools, config, context, version, metadata, created_at, updated_at
FROM assistants
WHERE ($1::jsonb IS NULL OR metadata @> $1::jsonb)
ORDER BY created_at DESC
LIMIT $2 OFFSET $3
`, jsonbOrNil(req.Metadata), intOr(req.Limit, 20), intOr(req.Offset, 0))
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	list, err := pgx.CollectRows(rows, pgx.RowToStructByName[assistantRow])
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	out := make([]Assistant, len(list))
	for i := range list {
		out[i] = list[i].toAPI()
	}
	return c.JSON(http.StatusOK, out)
}

// AssistantsCount — POST /assistants/count  (kind: read)
//   - SELECT count(*) FROM assistants WHERE metadata @> :filter
func (s *Server) AssistantsCount(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	var req AssistantCountRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	var n int
	if err := s.Tenant.QueryRow(ctx, `SELECT count(*) FROM assistants
WHERE ($1::jsonb IS NULL OR metadata @> $1::jsonb)
`, jsonbOrNil(req.Metadata)).Scan(&n); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, n)
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
	rows, err := s.Tenant.Query(ctx, `SELECT id, graph_id, name, description, model, instructions, tools, config, context, version, metadata, created_at, updated_at
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

// AssistantsUpdate — PATCH /assistants/{id}  (kind: write)
//   - INSERT events: event_type='assistant.updated', payload={changed fields}
//   - INSERT outbox (same event_id, same TX)
//   - pg_notify('outbox_new',”)
//   - UPDATE assistants SET name=:name, model=:model, ... WHERE id=:id
func (s *Server) AssistantsUpdate(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	pathID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	var req AssistantPatch
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	payload := mustJSON(req)
	events := []Event{
		{AggregateType: "Assistant", AggregateID: pathID, EventType: "assistant.updated", Payload: payload},
	}
	var row assistantRow
	if err := s.writeTx(ctx, s.Tenant, events, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `UPDATE assistants SET
  graph_id = COALESCE($2, graph_id),
  name = COALESCE($3, name),
  description = COALESCE($4, description),
  config = COALESCE($5, config),
  context = COALESCE($6, context),
  metadata = COALESCE($7, metadata)
WHERE id = $1
RETURNING id, graph_id, name, description, model, instructions, tools, config, context, version, metadata, created_at, updated_at
`, pathID, req.GraphId, req.Name, req.Description, jsonbOrNil(req.Config), jsonbOrNil(req.Context), jsonbOrNil(req.Metadata))
		if err != nil {
			return err
		}
		row, err = pgx.CollectOneRow(rows, pgx.RowToStructByName[assistantRow])
		return err
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, row.toAPI())
}

// AssistantsDelete — DELETE /assistants/{id}  (kind: write)
//   - INSERT events: event_type='assistant.deleted'
//   - INSERT outbox (same event_id, same TX)
//   - pg_notify('outbox_new',”)
//   - DELETE assistants WHERE id = :id (CASCADE → graphs)
func (s *Server) AssistantsDelete(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	pathID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	events := []Event{
		{AggregateType: "Assistant", AggregateID: pathID, EventType: "assistant.deleted"},
	}
	if err := s.writeTx(ctx, s.Tenant, events, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM assistants WHERE id = $1`, pathID)
		return err
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusOK)
}

// AssistantsGetGraph — GET /assistants/{id}/graph  (kind: read) — hand-written in assistants.go

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
