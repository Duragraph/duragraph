// Hand-written assistant create. Route is generated into assistants_gen.go
// (create marked custom in endpoints.yaml); body lives here. POST /assistants
// implements the LangGraph-Cloud idempotent-create contract, symmetric to
// threads_create.go: a client MAY supply assistant_id to make creation
// idempotent, and if_exists selects what happens on a clash — "raise" (the
// default) returns 409, "do_nothing" returns the existing assistant. When
// assistant_id is omitted a fresh id is minted and creation always succeeds. A
// single assistant.created event is emitted through the transactional-outbox
// path only on an actual insert; a no-op (do_nothing) hit and a 409 emit nothing.
package endpoints

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

// errAssistantConflict is a sentinel returned from inside the write TX when the
// INSERT hit ON CONFLICT DO NOTHING (row already exists). Returning an error
// rolls the TX back so the pre-appended assistant.created event is discarded —
// an idempotent create must not emit a second creation event for the same id.
var errAssistantConflict = errors.New("assistant already exists")

// assistantSelectColumns is the full column list returned by every assistant
// read/write, kept in one place so create's INSERT ... RETURNING and the
// conflict-path SELECT stay in lockstep with assistantRow.
const assistantSelectColumns = `id, graph_id, name, description, model, instructions, tools, config, context, version, metadata, created_at, updated_at`

// AssistantsCreate creates an assistant, honoring a client-supplied assistant_id
// and the if_exists policy. POST /assistants -> 201 (created) / 200 (existing,
// do_nothing) / 409 (existing, raise).
func (s *Server) AssistantsCreate(c echo.Context) error {
	ctx := c.Request().Context()
	var req AssistantCreate
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	// Honor a client-supplied assistant_id (makes create idempotent); otherwise
	// mint one. gen_random_uuid() on the column would ignore the client's id, so
	// the id is chosen here and passed explicitly.
	id := uuid.New()
	if req.AssistantId != nil {
		id = *req.AssistantId
	}

	events := []Event{
		{AggregateType: "Assistant", AggregateID: id, EventType: "assistant.created", Payload: mustJSON(req)},
	}

	var (
		out        assistantRow
		conflicted bool
	)
	err := s.writeTx(ctx, s.Tenant, events, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			INSERT INTO assistants (id, graph_id, name, description, config, context, metadata)
			VALUES ($1, $2, $3, $4, $5, COALESCE($6::jsonb, '{}'::jsonb), $7)
			ON CONFLICT (id) DO NOTHING
			RETURNING `+assistantSelectColumns,
			id, req.GraphId, deref(req.Name), req.Description, mustJSON(req.Config), jsonbOrNil(req.Context), mustJSON(req.Metadata))
		if err != nil {
			return err
		}
		out, err = pgx.CollectOneRow(rows, pgx.RowToStructByName[assistantRow])
		if errors.Is(err, pgx.ErrNoRows) {
			// Row already exists (ON CONFLICT DO NOTHING returned nothing). Abort
			// the TX so the assistant.created event is rolled back; the if_exists
			// policy is applied outside the TX.
			conflicted = true
			return errAssistantConflict
		}
		if err != nil {
			return err
		}
		// Snapshot the freshly-inserted assistant as version 1 (the column
		// default) into the history table, in the same TX as the insert.
		return snapshotAssistant(ctx, tx, id)
	})
	if conflicted {
		return s.assistantCreateConflict(c, ctx, id, req.IfExists)
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusCreated, out.toAPI())
}

// assistantCreateConflict resolves an assistant_id collision per the if_exists
// policy: "do_nothing" returns the pre-existing assistant (200, no event
// emitted); anything else — "raise" or an omitted policy — is a 409.
// LangGraph-Cloud defaults if_exists to "raise", so nil maps to 409.
func (s *Server) assistantCreateConflict(c echo.Context, ctx context.Context, id uuid.UUID, ifExists *AssistantCreateIfExists) error {
	if ifExists == nil || *ifExists != AssistantCreateIfExistsDoNothing {
		return echo.NewHTTPError(http.StatusConflict, "assistant already exists")
	}
	rows, err := s.Tenant.Query(ctx, `SELECT `+assistantSelectColumns+` FROM assistants WHERE id = $1`, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	existing, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[assistantRow])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Raced: the conflicting row was deleted between our INSERT and this
			// SELECT. Report a conflict rather than fabricate an assistant.
			return echo.NewHTTPError(http.StatusConflict, "assistant already exists")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, existing.toAPI())
}
