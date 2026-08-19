// Hand-written assistant update (route generated into assistants_gen.go via
// custom: true). PATCH /assistants/{id} mints a NEW assistant version on every
// update: it COALESCE-patches the supplied fields, sets the live version to
// MAX(assistant_versions.version)+1, snapshots the result into the history
// table, and emits assistant.updated — all in one transactional-outbox write.
// The generated update mode did a bare COALESCE UPDATE with no version bump and
// no snapshot, which the LangGraph-Cloud versioning contract requires.
package endpoints

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

// AssistantsUpdate — PATCH /assistants/{id}  (kind: write)
func (s *Server) AssistantsUpdate(c echo.Context) error {
	ctx := c.Request().Context()
	pathID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	var req AssistantPatch
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	events := []Event{
		{AggregateType: "Assistant", AggregateID: pathID, EventType: "assistant.updated", Payload: mustJSON(req)},
	}
	var row assistantRow
	if err := s.writeTx(ctx, s.Tenant, events, func(tx pgx.Tx) error {
		// COALESCE keeps omitted fields; version jumps to one past the highest
		// version ever recorded for this assistant (so a rollback via set_latest
		// does not cause the next update to collide with an existing snapshot).
		rows, err := tx.Query(ctx, `
			UPDATE assistants SET
				graph_id = COALESCE($2, graph_id),
				name = COALESCE($3, name),
				description = COALESCE($4, description),
				config = COALESCE($5, config),
				context = COALESCE($6, context),
				metadata = COALESCE($7, metadata),
				version = `+assistantNextVersionExpr+`
			WHERE id = $1
			RETURNING `+assistantSelectColumns,
			pathID, req.GraphId, req.Name, req.Description,
			jsonbOrNil(req.Config), jsonbOrNil(req.Context), jsonbOrNil(req.Metadata))
		if err != nil {
			return err
		}
		row, err = pgx.CollectOneRow(rows, pgx.RowToStructByName[assistantRow])
		if err != nil {
			return err
		}
		// Snapshot the just-updated row at its new version, same TX.
		return snapshotAssistant(ctx, tx, pathID)
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, row.toAPI())
}
