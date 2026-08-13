// Hand-written thread create. Route is generated into threads_gen.go (create
// marked custom in endpoints.yaml); body lives here. POST /threads implements
// the LangGraph-Cloud idempotent-create contract: a client MAY supply thread_id
// to make creation idempotent, and if_exists selects what happens on a clash —
// "raise" (the default) returns 409, "do_nothing" returns the existing thread.
// When thread_id is omitted a fresh id is minted and creation always succeeds.
// A single thread.created event is emitted through the transactional-outbox path
// only on an actual insert; a no-op (do_nothing) hit and a 409 emit nothing.
package endpoints

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

// errThreadConflict is a sentinel returned from inside the write TX when the
// INSERT hit ON CONFLICT DO NOTHING (row already exists). Returning an error
// rolls the TX back so the pre-appended thread.created event is discarded — an
// idempotent create must not emit a second creation event for the same id.
var errThreadConflict = errors.New("thread already exists")

// ThreadsCreate creates a thread, honoring a client-supplied thread_id and the
// if_exists policy. POST /threads -> 201 (created) / 200 (existing, do_nothing) /
// 409 (existing, raise).
func (s *Server) ThreadsCreate(c echo.Context) error {
	ctx := c.Request().Context()
	var req ThreadCreate
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	// Honor a client-supplied thread_id (makes create idempotent); otherwise mint
	// one. gen_random_uuid() on the column would ignore the client's id, so the id
	// is chosen here and passed explicitly.
	id := uuid.New()
	if req.ThreadId != nil {
		id = *req.ThreadId
	}

	events := []Event{
		{AggregateType: "Thread", AggregateID: id, EventType: "thread.created", Payload: mustJSON(req)},
	}

	var (
		out        threadRow
		conflicted bool
	)
	err := s.writeTx(ctx, s.Tenant, events, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			INSERT INTO threads (id, metadata)
			VALUES ($1, $2)
			ON CONFLICT (id) DO NOTHING
			RETURNING id, status, values, config, metadata, created_at, updated_at`,
			id, mustJSON(req.Metadata))
		if err != nil {
			return err
		}
		out, err = pgx.CollectOneRow(rows, pgx.RowToStructByName[threadRow])
		if errors.Is(err, pgx.ErrNoRows) {
			// Row already exists (ON CONFLICT DO NOTHING returned nothing). Abort
			// the TX so the thread.created event is rolled back; the if_exists
			// policy is applied outside the TX.
			conflicted = true
			return errThreadConflict
		}
		return err
	})
	if conflicted {
		return s.threadCreateConflict(c, ctx, id, req.IfExists)
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusCreated, out.toAPI())
}

// threadCreateConflict resolves a thread_id collision per the if_exists policy:
// "do_nothing" returns the pre-existing thread (200, no event emitted); anything
// else — "raise" or an omitted policy — is a 409. LangGraph-Cloud defaults
// if_exists to "raise", so nil maps to 409.
func (s *Server) threadCreateConflict(c echo.Context, ctx context.Context, id uuid.UUID, ifExists *ThreadCreateIfExists) error {
	if ifExists == nil || *ifExists != ThreadCreateIfExistsDoNothing {
		return echo.NewHTTPError(http.StatusConflict, "thread already exists")
	}
	rows, err := s.Tenant.Query(ctx, `
		SELECT id, status, values, config, metadata, created_at, updated_at
		FROM threads WHERE id = $1`, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	existing, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[threadRow])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Raced: the conflicting row was deleted between our INSERT and this
			// SELECT. Report a conflict rather than fabricate a thread.
			return echo.NewHTTPError(http.StatusConflict, "thread already exists")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, existing.toAPI())
}
