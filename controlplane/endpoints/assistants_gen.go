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
	g.POST("/assistants/:id/versions", s.AssistantsGetVersions)
	g.POST("/assistants/:id/latest", s.AssistantsSetLatest)
}

// AssistantsCreate — POST /assistants  (kind: write) — hand-written in assistants.go

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

// AssistantsUpdate — PATCH /assistants/{id}  (kind: write) — hand-written in assistants.go

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

// AssistantsGetVersions — POST /assistants/{id}/versions  (kind: read) — hand-written in assistants.go

// AssistantsSetLatest — POST /assistants/{id}/latest  (kind: write) — hand-written in assistants.go
