// Assistant version history — write-path snapshotting (snapshotAssistant, used
// by create/update) plus the read/rollback handlers get_versions and set_latest.
//
// LangGraph-Cloud assistant versioning: the live assistants row holds the
// currently ACTIVE version; every create and update appends an immutable
// snapshot of the assistant's editable state to assistant_versions, keyed by
// (assistant_id, version). set_latest re-points the live row to an older
// snapshot without minting a new version, so the "next" version an update mints
// is MAX(assistant_versions.version)+1, not live.version+1 — see
// assistantNextVersionExpr.
package endpoints

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

// assistantVersionColumns is the editable-state column set copied between the
// live assistants row and an assistant_versions snapshot, kept in one place so
// the snapshot INSERT and (PR-B) the set_latest copy-back stay in lockstep.
const assistantVersionColumns = `graph_id, name, description, model, instructions, tools, config, context, metadata`

// assistantNextVersionExpr is the scalar subquery that yields the next version
// number to mint for an assistant: one past the highest version ever recorded
// in its history (COALESCE handles the impossible "no snapshots yet" case as 1).
// Using the history MAX rather than the live assistants.version keeps update
// correct after a set_latest rollback re-points the live version backwards.
const assistantNextVersionExpr = `(SELECT COALESCE(MAX(version), 0) + 1 FROM assistant_versions WHERE assistant_id = $1)`

// snapshotAssistant appends an immutable snapshot of the assistant's current
// editable state to assistant_versions at its current live version. Called
// inside the same write TX as the create/update that produced that state, so
// the snapshot and the live row commit atomically. Idempotent under redelivery:
// re-snapshotting an existing (assistant_id, version) overwrites it rather than
// erroring, which cannot lose data because the source is the just-written live
// row.
func snapshotAssistant(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO assistant_versions (assistant_id, version, `+assistantVersionColumns+`)
		SELECT id, version, `+assistantVersionColumns+`
		FROM assistants WHERE id = $1
		ON CONFLICT (assistant_id, version) DO UPDATE SET
			graph_id = EXCLUDED.graph_id,
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			model = EXCLUDED.model,
			instructions = EXCLUDED.instructions,
			tools = EXCLUDED.tools,
			config = EXCLUDED.config,
			context = EXCLUDED.context,
			metadata = EXCLUDED.metadata`, id)
	return err
}

// assistantVersionSelectColumns projects an assistant_versions snapshot row onto
// the 13 columns assistantRow expects (RowToStructByName matches by output
// name). The history table keys on assistant_id and has no updated_at, so id is
// aliased from assistant_id and updated_at from created_at (a snapshot is
// immutable, so its "updated" instant is its creation instant).
const assistantVersionSelectColumns = `assistant_id AS id, graph_id, name, description, model, instructions, tools, config, context, version, metadata, created_at, created_at AS updated_at`

// assistantSelectColumnsA is assistantSelectColumns with every column qualified
// to the target-table alias `a`, for the UPDATE assistants a ... FROM
// assistant_versions v ... RETURNING in set_latest: the editable columns exist
// in both tables, so an unqualified RETURNING list is ambiguous.
const assistantSelectColumnsA = `a.id, a.graph_id, a.name, a.description, a.model, a.instructions, a.tools, a.config, a.context, a.version, a.metadata, a.created_at, a.updated_at`

// AssistantsGetVersions — POST /assistants/{id}/versions  (kind: read)
//
// Returns the assistant's version history newest-first as a list of Assistant
// snapshots. The OpenAPI contract declares only the path param, but the endpoint
// accepts an optional AssistantVersionsSearchRequest body (metadata exact-match
// filter + limit/offset) as a superset — an empty body binds to the zero value
// and returns the most recent `limit` versions.
func (s *Server) AssistantsGetVersions(c echo.Context) error {
	ctx := c.Request().Context()
	pathID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	var req AssistantVersionsSearchRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	rows, err := s.Tenant.Query(ctx, `
		SELECT `+assistantVersionSelectColumns+`
		FROM assistant_versions
		WHERE assistant_id = $1
		  AND ($2::jsonb IS NULL OR metadata @> $2::jsonb)
		ORDER BY version DESC
		LIMIT $3 OFFSET $4`,
		pathID, jsonbOrNil(req.Metadata), intOr(req.Limit, 10), intOr(req.Offset, 0))
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	list, err := pgx.CollectRows(rows, pgx.RowToStructByName[assistantRow])
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	out := make([]Assistant, len(list))
	for i := range list {
		out[i] = list[i].toAPI()
	}
	return c.JSON(http.StatusOK, out)
}

// AssistantsSetLatest — POST /assistants/{id}/latest?version=N  (kind: write)
//
// Re-points the live assistant to an existing historical version: it copies that
// snapshot's editable state back onto the live row and sets version = N WITHOUT
// minting a new version (the snapshot already exists; a later update mints
// MAX(history)+1, so no collision). The `version` query param is required and
// integer (422 otherwise); a missing assistant OR missing snapshot is 404. The
// re-point is an assistant mutation, so it emits assistant.updated through the
// transactional-outbox path in the same TX as the UPDATE.
func (s *Server) AssistantsSetLatest(c echo.Context) error {
	ctx := c.Request().Context()
	pathID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	version, err := strconv.Atoi(c.QueryParam("version"))
	if err != nil {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "version must be an integer")
	}
	events := []Event{
		{AggregateType: "Assistant", AggregateID: pathID, EventType: "assistant.updated", Payload: mustJSON(AssistantVersionChange{Version: &version})},
	}
	var row assistantRow
	if err := s.writeTx(ctx, s.Tenant, events, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			UPDATE assistants a SET
				graph_id = v.graph_id,
				name = v.name,
				description = v.description,
				model = v.model,
				instructions = v.instructions,
				tools = v.tools,
				config = v.config,
				context = v.context,
				metadata = v.metadata,
				version = v.version
			FROM assistant_versions v
			WHERE a.id = $1 AND v.assistant_id = $1 AND v.version = $2
			RETURNING `+assistantSelectColumnsA,
			pathID, version)
		if err != nil {
			return err
		}
		row, err = pgx.CollectOneRow(rows, pgx.RowToStructByName[assistantRow])
		return err
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// No assistant with that id, or no snapshot at that version — either way
			// there is nothing to activate. The pre-appended event rolls back.
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, row.toAPI())
}
