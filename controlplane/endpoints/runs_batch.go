// Hand-written batch run creation. Route is generated into runs_gen.go
// (batch_create marked custom in endpoints.yaml); body lives here. POST
// /runs/batch takes a RunBatchCreate ([]RunCreateStateless) and creates every
// run in one transaction: N run.created events + N runs-projection inserts + a
// single pg_notify wake-up, all atomic. The response schema is unconstrained
// ({}), so we return the created runs ([]Run) — the useful result and what the
// LangGraph SDK expects.
package endpoints

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

// RunsBatchCreate creates a batch of stateless runs atomically.
// POST /runs/batch -> 200 []Run.
func (s *Server) RunsBatchCreate(c echo.Context) error {
	ctx := c.Request().Context()
	var req RunBatchCreate
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if len(req) == 0 {
		return c.JSON(http.StatusOK, []Run{})
	}

	// Resolve every assistant reference (UUID or graph name) up front so an
	// unknown graph fails the whole batch before any event is appended.
	assistantIDs := make([]uuid.UUID, len(req))
	for i, r := range req {
		aid, err := s.resolveAssistantRef(ctx, r.AssistantId)
		if err != nil {
			return assistantRefHTTPError(err)
		}
		assistantIDs[i] = aid
	}

	// Pre-mint an id per run so the run.created events (built before the TX
	// projection) and the projection inserts agree on ids.
	ids := make([]uuid.UUID, len(req))
	events := make([]Event, len(req))
	for i, r := range req {
		ids[i] = uuid.New()
		events[i] = Event{
			AggregateType: "Run",
			AggregateID:   ids[i],
			EventType:     "run.created",
			Payload:       mustJSON(r),
		}
	}

	out := make([]Run, 0, len(req))
	if err := s.writeTx(ctx, s.Tenant, events, func(tx pgx.Tx) error {
		for i, r := range req {
			rows, err := tx.Query(ctx, `INSERT INTO runs (id, assistant_id, status, input, metadata)
VALUES ($1, $2, 'queued', $3, $4)
RETURNING `+runReturningColumns, ids[i], assistantIDs[i], mustJSON(r.Input), mustJSON(r.Metadata))
			if err != nil {
				return err
			}
			row, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[runRow])
			if err != nil {
				return err
			}
			out = append(out, row.toAPI())
		}
		return nil
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, out)
}
