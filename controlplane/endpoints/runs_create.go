// Hand-written run creation. Routes are generated into runs_gen.go
// (create_on_thread + create_stateless marked custom in endpoints.yaml); bodies
// live here. Both POST /threads/{id}/runs and POST /runs accept an assistant_id
// that is EITHER a UUID or a graph name (LangGraph-Cloud contract: "The
// assistant ID or graph name to run. If using graph name, will default to first
// assistant created from that graph."). The generated write_returning body could
// only pass the id straight through, so a graph-name string coerced to the zero
// UUID and the runs.assistant_id FK rejected it with a 500. These custom bodies
// resolve the reference to a real assistant id first (see resolveAssistantRef),
// then take the same transactional-outbox insert path as before.
package endpoints

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

// runReturningColumns is the full runs column list returned by every run insert,
// kept in one place so create_on_thread, create_stateless, and batch_create all
// scan the identical shape into runRow (strict RowToStructByName).
const runReturningColumns = `id, thread_id, assistant_id, status, input, output, error, metadata, kwargs, multitask_strategy, version, lease_epoch, worker_id, priority, graph_id, created_at, started_at, completed_at, updated_at`

// errAssistantRefNotFound is returned by resolveAssistantRef when an assistant_id
// given as a graph name matches no assistant. errAssistantRefInvalid is returned
// when the reference is neither a UUID nor a string (a malformed request body).
var (
	errAssistantRefNotFound = errors.New("no assistant found for graph")
	errAssistantRefInvalid  = errors.New("assistant_id must be a UUID or graph name")
)

// resolveAssistantRef resolves an OpenAPI interface{} assistant_id to a concrete
// assistants.id. A UUID (or UUID-shaped string) passes through unchanged — its
// existence is left to the runs.assistant_id FK, matching the prior behavior. A
// non-UUID string is treated as a graph name and resolved to the FIRST assistant
// created from that graph (created_at ASC), per the LangGraph-Cloud contract; no
// match is errAssistantRefNotFound. Anything else is errAssistantRefInvalid.
func (s *Server) resolveAssistantRef(ctx context.Context, ref interface{}) (uuid.UUID, error) {
	switch t := ref.(type) {
	case uuid.UUID:
		return t, nil
	case string:
		if u, err := uuid.Parse(t); err == nil {
			return u, nil
		}
		var id uuid.UUID
		err := s.Tenant.QueryRow(ctx,
			`SELECT id FROM assistants WHERE graph_id = $1 ORDER BY created_at ASC LIMIT 1`, t).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, errAssistantRefNotFound
		}
		if err != nil {
			return uuid.Nil, err
		}
		return id, nil
	default:
		return uuid.Nil, errAssistantRefInvalid
	}
}

// assistantRefHTTPError maps a resolveAssistantRef failure to the response the
// client should see: an unknown graph name is a 404 (the referenced resource
// does not exist), a malformed reference is a 422, and anything else is a 500.
func assistantRefHTTPError(err error) error {
	switch {
	case errors.Is(err, errAssistantRefNotFound):
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	case errors.Is(err, errAssistantRefInvalid):
		return echo.NewHTTPError(http.StatusUnprocessableEntity, err.Error())
	default:
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
}

// RunsCreateOnThread creates a stateful run on a thread. POST /threads/{id}/runs
// -> 201 Run. Resolves assistant_id (UUID or graph name) before inserting.
func (s *Server) RunsCreateOnThread(c echo.Context) error {
	ctx := c.Request().Context()
	pathID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	var req RunCreateStateful
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	assistantID, err := s.resolveAssistantRef(ctx, req.AssistantId)
	if err != nil {
		return assistantRefHTTPError(err)
	}
	// Validate the run-level interrupt spec BEFORE the write, so a malformed
	// one 422s without appending an event that would have to be rolled back.
	kwargs, err := buildRunKwargs(req.InterruptBefore, req.InterruptAfter)
	if err != nil {
		return interruptSpecHTTPError(err)
	}

	aggID := uuid.New()
	events := []Event{
		{AggregateType: "Run", AggregateID: aggID, EventType: "run.created", Payload: mustJSON(req)},
	}
	var row runRow
	if err := s.writeTx(ctx, s.Tenant, events, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `INSERT INTO runs (id, thread_id, assistant_id, status, input, metadata, kwargs)
VALUES ($1, $2, $3, 'queued', $4, $5, $6)
RETURNING `+runReturningColumns,
			aggID, pathID, assistantID, mustJSON(req.Input), mustJSON(req.Metadata), kwargs)
		if err != nil {
			return err
		}
		row, err = pgx.CollectOneRow(rows, pgx.RowToStructByName[runRow])
		return err
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusCreated, row.toAPI())
}

// RunsCreateStateless creates a stateless run. POST /runs -> 201 Run. Resolves
// assistant_id (UUID or graph name) before inserting.
func (s *Server) RunsCreateStateless(c echo.Context) error {
	ctx := c.Request().Context()
	var req RunCreateStateless
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	assistantID, err := s.resolveAssistantRef(ctx, req.AssistantId)
	if err != nil {
		return assistantRefHTTPError(err)
	}
	kwargs, err := buildRunKwargs(req.InterruptBefore, req.InterruptAfter)
	if err != nil {
		return interruptSpecHTTPError(err)
	}

	aggID := uuid.New()
	events := []Event{
		{AggregateType: "Run", AggregateID: aggID, EventType: "run.created", Payload: mustJSON(req)},
	}
	var row runRow
	if err := s.writeTx(ctx, s.Tenant, events, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `INSERT INTO runs (id, assistant_id, status, input, metadata, kwargs)
VALUES ($1, $2, 'queued', $3, $4, $5)
RETURNING `+runReturningColumns,
			aggID, assistantID, mustJSON(req.Input), mustJSON(req.Metadata), kwargs)
		if err != nil {
			return err
		}
		row, err = pgx.CollectOneRow(rows, pgx.RowToStructByName[runRow])
		return err
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusCreated, row.toAPI())
}
