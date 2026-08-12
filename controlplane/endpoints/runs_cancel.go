// Hand-written batch/stateless run cancellation. Route is generated into
// runs_gen.go (cancel_stateless marked custom in endpoints.yaml); body lives
// here. POST /runs/cancel takes a RunsCancel selector — either an explicit
// run_ids list or a status filter (pending/running/all), optionally narrowed to
// one thread — cancels the matching in-flight runs, and returns 204 (the
// LangGraph-Cloud contract; no body). One run.cancelled event is emitted per
// run that is actually transitioned, through the transactional-outbox path.
package endpoints

import (
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

// cancellableStatuses are the runs.status values a cancel may transition from.
// A run already completed/failed/cancelled is left untouched.
const cancellableStatuses = "'queued', 'in_progress', 'requires_action'"

// cancelStatusToDB maps an API RunsCancel.status filter to the DB run statuses
// it selects. Returns nil for an unrecognized value (→ 422).
func cancelStatusToDB(status string) []string {
	switch status {
	case "pending":
		return []string{"queued"}
	case "running":
		return []string{"in_progress"}
	case "all":
		return []string{"queued", "in_progress", "requires_action"}
	default:
		return nil
	}
}

// RunsCancelStateless cancels a set of runs selected by run_ids or status
// filter. POST /runs/cancel -> 204 / 422 (no selector) / 500.
//
// Selection and mutation are two statements (resolve cancellable ids, then
// UPDATE). Both carry the cancellable-status guard, so a run that reaches a
// terminal state in the gap between them is not clobbered; the only residue is
// a run.cancelled event for a run that finished first, which is benign for an
// idempotent cancel. Events are built from the resolved set because writeTx
// appends them before running the projection.
func (s *Server) RunsCancelStateless(c echo.Context) error {
	ctx := c.Request().Context()
	var req RunsCancel
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	where := "status IN (" + cancellableStatuses + ")"
	args := []any{}
	n := 1
	switch {
	case req.RunIds != nil && len(*req.RunIds) > 0:
		where += fmt.Sprintf(" AND id = ANY($%d)", n)
		args = append(args, *req.RunIds)
		n++
	case req.Status != nil:
		dbStatuses := cancelStatusToDB(string(*req.Status))
		if dbStatuses == nil {
			return echo.NewHTTPError(http.StatusUnprocessableEntity, "invalid status filter")
		}
		where += fmt.Sprintf(" AND status = ANY($%d)", n)
		args = append(args, dbStatuses)
		n++
	default:
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "must provide run_ids or status")
	}
	if req.ThreadId != nil {
		where += fmt.Sprintf(" AND thread_id = $%d", n)
		args = append(args, *req.ThreadId)
		n++
	}

	rows, err := s.Tenant.Query(ctx, "SELECT id FROM runs WHERE "+where, args...)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if len(ids) == 0 {
		// Nothing in flight matched — cancel is idempotent, so this is success.
		return c.NoContent(http.StatusNoContent)
	}

	events := make([]Event, len(ids))
	for i, id := range ids {
		events[i] = Event{AggregateType: "Run", AggregateID: id, EventType: "run.cancelled"}
	}
	if err := s.writeTx(ctx, s.Tenant, events, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE runs SET status='cancelled', version=version+1, updated_at=now()
WHERE id = ANY($1) AND status IN (`+cancellableStatuses+`)`, ids)
		return err
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}
