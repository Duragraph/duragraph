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
	pathID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	var req CronCreate
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	var row cronRow
	rows, err := s.Tenant.Query(ctx, `INSERT INTO crons (thread_id, assistant_id, schedule, input, metadata)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, thread_id, assistant_id, schedule, input, config, metadata, end_time, user_id, next_run_at, created_at, updated_at
`, pathID, asUUID(req.AssistantId), req.Schedule, mustJSON(req.Input), mustJSON(req.Metadata))
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	row, err = pgx.CollectOneRow(rows, pgx.RowToStructByName[cronRow])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, row.toAPI())
}

// CronsList — GET /runs/crons  (kind: read)
//   - SELECT * FROM crons ORDER BY created_at DESC
func (s *Server) CronsList(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	rows, err := s.Tenant.Query(ctx, `SELECT id, thread_id, assistant_id, schedule, input, config, metadata, end_time, user_id, next_run_at, created_at, updated_at
FROM crons ORDER BY created_at DESC
`)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	list, err := pgx.CollectRows(rows, pgx.RowToStructByName[cronRow])
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	out := make([]Cron, len(list))
	for i := range list {
		out[i] = list[i].toAPI()
	}
	return c.JSON(http.StatusOK, out)
}

// CronsSearch — POST /runs/crons/search  (kind: read)
//   - SELECT * FROM crons WHERE metadata @> :filter
func (s *Server) CronsSearch(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	var req CronSearch
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	rows, err := s.Tenant.Query(ctx, `SELECT id, thread_id, assistant_id, schedule, input, config, metadata, end_time, user_id, next_run_at, created_at, updated_at
FROM crons
WHERE ($1::uuid IS NULL OR assistant_id = $1)
  AND ($2::uuid IS NULL OR thread_id = $2)
ORDER BY created_at DESC
LIMIT $3 OFFSET $4
`, req.AssistantId, req.ThreadId, intOr(req.Limit, 20), intOr(req.Offset, 0))
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	list, err := pgx.CollectRows(rows, pgx.RowToStructByName[cronRow])
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	out := make([]Cron, len(list))
	for i := range list {
		out[i] = list[i].toAPI()
	}
	return c.JSON(http.StatusOK, out)
}

// CronsCount — POST /runs/crons/count  (kind: read)
//   - SELECT count(*) FROM crons WHERE ...
func (s *Server) CronsCount(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	var req CronCountRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	var n int
	if err := s.Tenant.QueryRow(ctx, `SELECT count(*) FROM crons
WHERE ($1::uuid IS NULL OR assistant_id = $1)
  AND ($2::uuid IS NULL OR thread_id = $2)
`, req.AssistantId, req.ThreadId).Scan(&n); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, n)
}

// CronsGet — GET /runs/crons/{cron_id}  (kind: read)
//   - SELECT * FROM crons WHERE id = :cron_id
func (s *Server) CronsGet(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	pathID, err := uuid.Parse(c.Param("cron_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid cron_id")
	}
	rows, err := s.Tenant.Query(ctx, `SELECT id, thread_id, assistant_id, schedule, input, config, metadata, end_time, user_id, next_run_at, created_at, updated_at
FROM crons WHERE id = $1
`, pathID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	row, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[cronRow])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, row.toAPI())
}

// CronsDelete — DELETE /runs/crons/{cron_id}  (kind: delete)
//   - DELETE crons WHERE id = :cron_id
func (s *Server) CronsDelete(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	pathID, err := uuid.Parse(c.Param("cron_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid cron_id")
	}
	ct, err := s.Tenant.Exec(ctx, `DELETE FROM crons WHERE id = $1`, pathID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if ct.RowsAffected() == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}
	return c.NoContent(http.StatusOK)
}
