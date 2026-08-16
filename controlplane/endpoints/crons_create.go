// Hand-written cron-create handler (route generated into crons_gen.go via
// custom: true). Crons are scheduler definitions, NOT event-sourced domain
// aggregates (outbox: false), so this is a plain INSERT — no writeTx/outbox.
// Unlike the generated body it replaced, it (a) resolves assistant_id as a
// UUID or graph name via resolveAssistantRef (the LangGraph-Cloud contract:
// CronCreate.assistant_id is "the assistant ID or graph name to run"), and
// (b) persists the config and end_time fields the contract advertises and the
// crons table has columns for. context/interrupt_*/multitask_strategy/webhook
// stay unhonored — no column, and (interrupt/multitask/webhook) no scheduler
// engine or delivery machinery yet (see DIVERGENCES in rows.go).
package endpoints

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

// cronReturningColumns is the full cronRow column set, shared by the create
// handler so the RETURNING list stays in lockstep with cronRow's strict
// RowToStructByName fields.
const cronReturningColumns = `id, thread_id, assistant_id, schedule, input, config, metadata, end_time, user_id, next_run_at, created_at, updated_at`

// CronsCreate — POST /threads/{id}/runs/crons  (kind: write)
func (s *Server) CronsCreate(c echo.Context) error {
	ctx := c.Request().Context()
	pathID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	var req CronCreate
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	assistantID, err := s.resolveAssistantRef(ctx, req.AssistantId)
	if err != nil {
		return assistantRefHTTPError(err)
	}
	rows, err := s.Tenant.Query(ctx, `INSERT INTO crons (thread_id, assistant_id, schedule, input, config, metadata, end_time)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING `+cronReturningColumns, pathID, assistantID, req.Schedule,
		mustJSON(req.Input), jsonbObjectOrEmpty(req.Config), jsonbObjectOrEmpty(req.Metadata), req.EndTime)
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
