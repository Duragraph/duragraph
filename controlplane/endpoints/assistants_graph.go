// Hand-written assistant graph read. Route is generated into assistants_gen.go
// (get_graph marked custom in endpoints.yaml); body lives here. See rows.go's
// graphRow.toAPI + DIVERGENCES for the GraphSchema hand-mapping.
package endpoints

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

// AssistantsGetGraph returns the latest graph definition for an assistant.
// GET /assistants/{id}/graph -> 200 GraphSchema / 404 (no assistant, or an
// assistant with no graph row).
func (s *Server) AssistantsGetGraph(c echo.Context) error {
	ctx := c.Request().Context()

	// assistant_id here is NOT uuid-only. The OpenAPI types it as
	// anyOf[{string,format:uuid,"Assistant ID"}, {string,"Graph ID"}], so a
	// non-UUID is a LEGAL request identifying the graph by name — not a
	// validation error. Interpolating it raw into `WHERE assistant_id = $1`
	// (a uuid column) made Postgres reject the legal form:
	//
	//	GET /assistants/my-graph/graph
	//	  500 {"message":"ERROR: invalid input syntax for type uuid: \"my-graph\" (SQLSTATE 22P02)"}
	//
	// So the branch is on which arm of the anyOf was sent: a UUID selects by
	// assistant, anything else by graphs.name (indexed, idx_graphs_name).
	// A name that matches nothing falls through to the same 404 as an unknown
	// assistant, which is the declared response.
	raw := c.Param("id")
	where, arg := "name = $1", raw
	if _, err := uuid.Parse(raw); err == nil {
		where, arg = "assistant_id = $1", raw
	}
	rows, err := s.Tenant.Query(ctx, `
		SELECT id, assistant_id, name, version, description, nodes, edges, config, created_at, updated_at
		FROM graphs
		WHERE `+where+`
		ORDER BY version DESC
		LIMIT 1`, arg)
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
