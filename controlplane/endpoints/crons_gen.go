// Code generated from controlplane/gen/endpoints.yaml — DO NOT EDIT.
// Source of truth: spec/models/d2/endpoint-queries.d2 (via endpoints.yaml).
// Regenerate: go run ./controlplane/gen
package endpoints

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// RegisterCrons mounts the crons endpoints on g (the /api/v1 group).
func (s *Server) RegisterCrons(g *echo.Group) {
	g.POST("/threads/:id/runs/crons", s.CronsCreate)
	g.GET("/runs/crons", s.CronsList)
	g.POST("/runs/crons/search", s.CronsSearch)
	g.POST("/runs/crons/count", s.CronsCount)
	g.GET("/runs/crons/:cron_id", s.CronsGet)
	g.DELETE("/runs/crons/:cron_id", s.CronsDelete)
}

// CronsCreate — POST /threads/{id}/runs/crons  (kind: write)
//   - INSERT crons (id, thread_id, assistant_id, schedule, input, config)  # cron def, not domain event
func (s *Server) CronsCreate(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	var req map[string]any // TODO: bind OpenAPI type (CronsCreate request schema)
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	// projection-only write (not event-sourced — no outbox):
	//   INSERT crons (id, thread_id, assistant_id, schedule, input, config)  # cron def, not domain event
	return c.JSON(http.StatusOK, map[string]any{}) // TODO: return OpenAPI response type
}

// CronsList — GET /runs/crons  (kind: read)
//   - SELECT * FROM crons ORDER BY created_at DESC
func (s *Server) CronsList(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	// TODO read query (no side effects):
	//   SELECT * FROM crons ORDER BY created_at DESC
	return c.JSON(http.StatusOK, map[string]any{})
}

// CronsSearch — POST /runs/crons/search  (kind: read)
//   - SELECT * FROM crons WHERE metadata @> :filter
func (s *Server) CronsSearch(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	// TODO read query (no side effects):
	//   SELECT * FROM crons WHERE metadata @> :filter
	return c.JSON(http.StatusOK, map[string]any{})
}

// CronsCount — POST /runs/crons/count  (kind: read)
//   - SELECT count(*) FROM crons WHERE ...
func (s *Server) CronsCount(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	// TODO read query (no side effects):
	//   SELECT count(*) FROM crons WHERE ...
	return c.JSON(http.StatusOK, map[string]any{})
}

// CronsGet — GET /runs/crons/{cron_id}  (kind: read)
//   - SELECT * FROM crons WHERE id = :cron_id
func (s *Server) CronsGet(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	// TODO read query (no side effects):
	//   SELECT * FROM crons WHERE id = :cron_id
	return c.JSON(http.StatusOK, map[string]any{})
}

// CronsDelete — DELETE /runs/crons/{cron_id}  (kind: delete)
//   - DELETE crons WHERE id = :cron_id
func (s *Server) CronsDelete(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	// TODO hard delete:
	//   DELETE crons WHERE id = :cron_id
	return c.NoContent(http.StatusNoContent)
}
