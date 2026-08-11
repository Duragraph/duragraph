// Hand-written thread copy. Route is generated into threads_gen.go (copy marked
// custom in endpoints.yaml); body lives here. POST /threads/{id}/copy takes no
// request body and returns the new Thread (LangGraph-Cloud contract). The copy
// carries the source thread's state (values), config, and metadata, plus its
// message history, under a fresh thread id; status resets to the default
// ('idle') since the copy has no in-flight run. A single thread.created event is
// emitted for the new thread through the transactional-outbox path.
package endpoints

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

// ThreadsCopy duplicates a thread (values/config/metadata + message history)
// under a new id. POST /threads/{id}/copy -> 200 Thread / 404 (source missing).
func (s *Server) ThreadsCopy(c echo.Context) error {
	ctx := c.Request().Context()
	srcID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	// Read the source thread's copyable columns up front (outside the write TX):
	// the thread.created payload records the source id + copied fields, and the
	// event slice must be built before writeTx runs the projection.
	var src threadRow
	rows, err := s.Tenant.Query(ctx, `
		SELECT id, status, values, config, metadata, created_at, updated_at
		FROM threads WHERE id = $1`, srcID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	src, err = pgx.CollectOneRow(rows, pgx.RowToStructByName[threadRow])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	newID := uuid.New()
	events := []Event{{
		AggregateType: "Thread",
		AggregateID:   newID,
		EventType:     "thread.created",
		Payload:       mustJSON(map[string]any{"copied_from": srcID}),
	}}

	var out threadRow
	if err := s.writeTx(ctx, s.Tenant, events, func(tx pgx.Tx) error {
		r, err := tx.Query(ctx, `
			INSERT INTO threads (id, values, config, metadata)
			VALUES ($1, $2, $3, $4)
			RETURNING id, status, values, config, metadata, created_at, updated_at`,
			newID, src.Values, jsonOrEmpty(src.Config), jsonOrEmpty(src.Metadata))
		if err != nil {
			return err
		}
		if out, err = pgx.CollectOneRow(r, pgx.RowToStructByName[threadRow]); err != nil {
			return err
		}
		// Copy message history with fresh ids under the new thread, preserving
		// role/content/metadata and original ordering (created_at carried over).
		_, err = tx.Exec(ctx, `
			INSERT INTO messages (thread_id, role, content, metadata, created_at)
			SELECT $1, role, content, metadata, created_at
			FROM messages WHERE thread_id = $2`, newID, srcID)
		return err
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, out.toAPI())
}
