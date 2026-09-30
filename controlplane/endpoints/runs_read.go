package endpoints

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

// RunsListAll lists runs across threads, including stateless runs. With no
// limit it returns the full list for the dashboard's global and assistant views.
// Optional limit/offset let other callers page through the same ordering.
func (s *Server) RunsListAll(c echo.Context) error {
	var assistantID *uuid.UUID
	if raw := c.QueryParam("assistant_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid assistant_id")
		}
		assistantID = &id
	}

	query := `SELECT id, thread_id, assistant_id, status, input, output, error, metadata, kwargs, multitask_strategy, version, lease_epoch, worker_id, priority, graph_id, created_at, started_at, completed_at, updated_at
FROM runs WHERE ($1::uuid IS NULL OR assistant_id = $1)
ORDER BY created_at DESC, id DESC`
	args := []any{assistantID}
	for _, param := range []string{"limit", "offset"} {
		if values, present := c.QueryParams()[param]; present {
			raw := values[0]
			n, err := strconv.Atoi(raw)
			if err != nil || n < 0 || (param == "limit" && n == 0) {
				return echo.NewHTTPError(http.StatusBadRequest, "invalid "+param)
			}
			args = append(args, n)
			query += "\n" + strings.ToUpper(param) + " $" + strconv.Itoa(len(args))
		}
	}

	rows, err := s.Tenant.Query(c.Request().Context(), query, args...)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	list, err := pgx.CollectRows(rows, pgx.RowToStructByName[runRow])
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	out := make([]Run, len(list))
	for i := range list {
		out[i] = list[i].toAPI()
	}
	return c.JSON(http.StatusOK, out)
}
