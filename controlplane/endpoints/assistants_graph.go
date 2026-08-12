// Hand-written assistant graph read. Route is generated into assistants_gen.go
// (get_graph marked custom in endpoints.yaml); body lives here. See rows.go's
// graphRow.toAPI + DIVERGENCES for the GraphSchema hand-mapping.
package endpoints

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

// AssistantsGetGraph returns the latest graph definition for an assistant.
// GET /assistants/{id}/graph -> 200 GraphSchema / 404 (no assistant, or an
// assistant with no graph row).
func (s *Server) AssistantsGetGraph(c echo.Context) error {
	ctx := c.Request().Context()
	aid := c.Param("id")
	rows, err := s.Tenant.Query(ctx, `
		SELECT id, assistant_id, name, version, description, nodes, edges, config, created_at, updated_at
		FROM graphs
		WHERE assistant_id = $1
		ORDER BY version DESC
		LIMIT 1`, aid)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	row, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[graphRow])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, row.toAPI())
}
