// Hand-written HITL resume. Route is generated into runs_gen.go (resume marked
// custom in endpoints.yaml); body lives here. POST
// /threads/{id}/runs/{rid}/resume takes a LangGraph-style Command (nested under
// `command`; see spec/models/d2/hitl.d2 ResumeRunRequest), resolves the run's
// open interrupt, and re-dispatches it:
//
//   - Validate: the run is thread-scoped to {id}, in status requires_action,
//     and has exactly one unresolved interrupt. Otherwise 404 (no such
//     paused run/interrupt).
//   - In one transactional-outbox write: append run.resumed{interrupt_id,
//     command}, mark the interrupt resolved, and flip the run back to
//     in_progress (version+1), each guarded so a concurrent resume that already
//     transitioned the run loses the race (0 rows → 409).
//
// The run.resumed event lands on the RUNS stream; run-processor turns it into a
// fresh worker.graph.execute command carrying the command as `resume` (deduped
// on the event id, not the run id — see nats/run_processor.go). The worker
// restores the pause checkpoint, applies command.update, and continues the walk.
package endpoints

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

// RunsResume resumes a run paused at a HITL interrupt.
// POST /threads/{id}/runs/{rid}/resume -> 200 / 400 / 404 / 409 / 500.
func (s *Server) RunsResume(c echo.Context) error {
	ctx := c.Request().Context()

	threadID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid thread id")
	}
	runID, err := uuid.Parse(c.Param("rid"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid run id")
	}

	// Body is a ResumeRunRequest; for this slice we carry only the nested
	// `command` through to the worker. An absent command resumes with no state
	// change (the worker applies command.update, which is then empty).
	var req struct {
		Command json.RawMessage `json:"command,omitempty"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	// Validate before the write: the run must be paused (requires_action),
	// scoped to this thread, with an open interrupt to resolve. This runs
	// outside the tx only to produce a clean 404; the writes below re-guard on
	// status so a concurrent resume cannot double-dispatch.
	var interruptID uuid.UUID
	err = s.Tenant.QueryRow(ctx, `
		SELECT i.id
		FROM runs r
		JOIN interrupts i ON i.run_id = r.id AND NOT i.resolved
		WHERE r.id = $1 AND r.thread_id = $2 AND r.status = 'requires_action'
		ORDER BY i.created_at DESC
		LIMIT 1`,
		runID, threadID).Scan(&interruptID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "no resumable interrupt for this run")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	command := req.Command
	if len(command) == 0 {
		command = json.RawMessage("{}")
	}
	payload := mustJSON(map[string]any{
		"interrupt_id": interruptID.String(),
		"command":      command,
	})
	events := []Event{{
		AggregateType: "Run",
		AggregateID:   runID,
		EventType:     "run.resumed",
		Payload:       payload,
	}}

	if err := s.writeTx(ctx, s.Tenant, events, func(tx pgx.Tx) error {
		ct, err := tx.Exec(ctx, `
			UPDATE interrupts SET resolved = true, resolved_at = now()
			WHERE id = $1 AND NOT resolved`, interruptID)
		if err != nil {
			return err
		}
		if ct.RowsAffected() == 0 {
			return errResumeConflict // interrupt resolved concurrently
		}
		ct, err = tx.Exec(ctx, `
			UPDATE runs SET status = 'in_progress', version = version + 1, updated_at = now()
			WHERE id = $1 AND status = 'requires_action'`, runID)
		if err != nil {
			return err
		}
		if ct.RowsAffected() == 0 {
			return errResumeConflict // run left requires_action concurrently
		}
		return nil
	}); err != nil {
		if errors.Is(err, errResumeConflict) {
			return echo.NewHTTPError(http.StatusConflict, "run is no longer resumable")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]any{
		"run_id":       runID.String(),
		"interrupt_id": interruptID.String(),
		"status":       "in_progress",
	})
}

// errResumeConflict signals that the run/interrupt was transitioned out from
// under this resume (a concurrent resume or cancel won the race) — surfaced as
// 409 so the writeTx rolls back atomically.
var errResumeConflict = errors.New("resume: run no longer in requires_action")
