package endpoints

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

// RunsListOnThread lists runs newest first. Both the full and selected responses
// use runRow.toAPI, so status and field names match GET /threads/:id/runs/:rid.
func (s *Server) RunsListOnThread(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	q := c.Request().URL.Query()
	parsePage := func(key string, fallback int) (int, error) {
		if !q.Has(key) {
			return fallback, nil
		}
		if len(q[key]) != 1 {
			return 0, errors.New("invalid " + key)
		}
		n, err := strconv.Atoi(q.Get(key))
		if err != nil || n < 0 {
			return 0, errors.New("invalid " + key)
		}
		return n, nil
	}
	limit, err := parsePage("limit", 10)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	offset, err := parsePage("offset", 0)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	status := q.Get("status")
	if len(q["status"]) > 1 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid status")
	}
	switch status {
	case "", "pending", "error", "success", "timeout", "interrupted":
	default:
		return echo.NewHTTPError(http.StatusBadRequest, "invalid status")
	}
	// select is an optional repeated/comma-separated query parameter. Keep only
	// documented Run fields, rather than interpolating client input into SQL.
	var selected map[string]bool
	if q.Has("select") {
		selected = make(map[string]bool)
		for _, value := range q["select"] {
			for _, field := range strings.Split(value, ",") {
				switch field {
				case "run_id", "thread_id", "assistant_id", "created_at", "updated_at", "status", "metadata", "kwargs", "multitask_strategy":
					selected[field] = true
				default:
					return echo.NewHTTPError(http.StatusBadRequest, "invalid select")
				}
			}
		}
	}
	ctx := c.Request().Context()
	var exists bool
	if err := s.Tenant.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM threads WHERE id=$1)`, id).Scan(&exists); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if !exists {
		return echo.NewHTTPError(http.StatusNotFound, "thread not found")
	}
	// Filter on the same API statuses returned by toAPI (including cancelled
	// as error). timeout has no corresponding persisted DB state today.
	rows, err := s.Tenant.Query(ctx, `SELECT id, thread_id, assistant_id, status, input, output, error, metadata, kwargs, multitask_strategy, version, lease_epoch, worker_id, priority, graph_id, created_at, started_at, completed_at, updated_at
FROM runs WHERE thread_id=$1 AND ($2='' OR CASE status
  WHEN 'queued' THEN 'pending' WHEN 'in_progress' THEN 'running'
  WHEN 'requires_action' THEN 'interrupted' WHEN 'completed' THEN 'success'
  WHEN 'failed' THEN 'error' WHEN 'cancelled' THEN 'error' END = $2)
ORDER BY created_at DESC, id DESC LIMIT $3 OFFSET $4`, id, status, limit, offset)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	list, err := pgx.CollectRows(rows, pgx.RowToStructByName[runRow])
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if selected == nil {
		out := make([]Run, 0, len(list))
		for _, row := range list {
			out = append(out, row.toAPI())
		}
		return c.JSON(http.StatusOK, out)
	}
	out := make([]map[string]any, 0, len(list))
	for _, row := range list {
		run := row.toAPI()
		fields := map[string]any{
			"run_id": run.RunId, "thread_id": run.ThreadId,
			"assistant_id": run.AssistantId, "created_at": run.CreatedAt,
			"updated_at": run.UpdatedAt, "status": run.Status,
			"metadata": run.Metadata, "kwargs": run.Kwargs,
			"multitask_strategy": run.MultitaskStrategy,
		}
		projection := make(map[string]any, len(selected))
		for field := range selected {
			projection[field] = fields[field]
		}
		out = append(out, projection)
	}
	return c.JSON(http.StatusOK, out)
}
