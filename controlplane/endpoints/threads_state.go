// Hand-written thread state/checkpoint/history reads. Routes are generated
// into threads_gen.go (get_state/get_checkpoint_state/get_history marked
// custom in endpoints.yaml); bodies live here. All three read the snapshots
// table scoped to the thread's own runs (aggregate_id IN (SELECT id FROM runs
// WHERE thread_id=...)) — mirrors WorkersReadCheckpoint's thread-scoping in
// workers.go, so a checkpoint belonging to another thread's run always 404s,
// never leaks. See rows.go's snapshotRow.toThreadState + DIVERGENCES for the
// ThreadState hand-mapping.
package endpoints

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

// ThreadsGetState returns the latest snapshot (highest bigserial id = most
// recently written) across the thread's runs. Ordering is by id, not version:
// snapshots.version is incremented per event-stream, so with more than one run
// on a thread it does not order globally — id (BIGSERIAL) is the true
// chronological key. GET /threads/{id}/state -> 200 ThreadState / 404.
func (s *Server) ThreadsGetState(c echo.Context) error {
	ctx := c.Request().Context()
	tid, err := pathUUID(c, "id")
	if err != nil {
		return err
	}
	rows, err := s.Tenant.Query(ctx, `
		SELECT id, stream_id, aggregate_id, version, state, created_at
		FROM snapshots
		WHERE aggregate_id IN (SELECT id FROM runs WHERE thread_id = $1)
		ORDER BY id DESC
		LIMIT 1`, tid)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	row, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[snapshotRow])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, row.toThreadState())
}

// ThreadsGetCheckpointState returns one snapshot by id, scoped to the
// thread's own runs — a checkpoint id from another thread's run 404s.
// GET /threads/{id}/state/{checkpoint_id} -> 200 ThreadState / 404.
func (s *Server) ThreadsGetCheckpointState(c echo.Context) error {
	ctx := c.Request().Context()
	tid, err := pathUUID(c, "id")
	if err != nil {
		return err
	}
	ckpt, err := parseCheckpointID(c.Param("checkpoint_id"))
	if err != nil {
		return err
	}
	rows, err := s.Tenant.Query(ctx, `
		SELECT id, stream_id, aggregate_id, version, state, created_at
		FROM snapshots
		WHERE id = $1 AND aggregate_id IN (SELECT id FROM runs WHERE thread_id = $2)`, ckpt, tid)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	row, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[snapshotRow])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, row.toThreadState())
}

// ThreadsCreateCheckpoint, despite its name, is a READ: the OpenAPI
// ThreadStateCheckpointRequest -> ThreadState contract is "get the state of a
// thread AT a checkpoint" (the checkpoint id travels in the body's
// CheckpointConfig, not the path, because a full checkpoint config is richer
// than a path param). It is the body-carried twin of ThreadsGetCheckpointState.
// The endpoint-queries.d2 "INSERT snapshots" step mismodels it as a write — see
// DIVERGENCES in rows.go. When checkpoint.checkpoint_id is absent we fall back
// to the latest snapshot (same as ThreadsGetState). All lookups are scoped to
// the thread's own runs so another thread's checkpoint 404s.
// POST /threads/{id}/state/checkpoint -> 200 ThreadState / 404.
func (s *Server) ThreadsCreateCheckpoint(c echo.Context) error {
	ctx := c.Request().Context()
	tid, err := pathUUID(c, "id")
	if err != nil {
		return err
	}
	var req ThreadStateCheckpointRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	var rows pgx.Rows
	if req.Checkpoint.CheckpointId != nil && *req.Checkpoint.CheckpointId != "" {
		// Body-carried twin of the path param, so it needs the same guard: a
		// CheckpointConfig.checkpoint_id that is not a checkpoint identifier is
		// a validation error, not a 500 from the driver.
		ckpt, perr := parseCheckpointID(*req.Checkpoint.CheckpointId)
		if perr != nil {
			return perr
		}
		rows, err = s.Tenant.Query(ctx, `
			SELECT id, stream_id, aggregate_id, version, state, created_at
			FROM snapshots
			WHERE id = $1 AND aggregate_id IN (SELECT id FROM runs WHERE thread_id = $2)`,
			ckpt, tid)
	} else {
		rows, err = s.Tenant.Query(ctx, `
			SELECT id, stream_id, aggregate_id, version, state, created_at
			FROM snapshots
			WHERE aggregate_id IN (SELECT id FROM runs WHERE thread_id = $1)
			ORDER BY id DESC
			LIMIT 1`, tid)
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	row, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[snapshotRow])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, row.toThreadState())
}

// ThreadsGetHistory returns every snapshot across the thread's runs,
// newest-first (ORDER BY id DESC — global write order, since version is
// per-stream). GET /threads/{id}/history -> 200 []ThreadState.
func (s *Server) ThreadsGetHistory(c echo.Context) error {
	ctx := c.Request().Context()
	tid, err := pathUUID(c, "id")
	if err != nil {
		return err
	}
	rows, err := s.Tenant.Query(ctx, `
		SELECT id, stream_id, aggregate_id, version, state, created_at
		FROM snapshots
		WHERE aggregate_id IN (SELECT id FROM runs WHERE thread_id = $1)
		ORDER BY id DESC`, tid)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	list, err := pgx.CollectRows(rows, pgx.RowToStructByName[snapshotRow])
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	out := make([]ThreadState, len(list))
	for i := range list {
		out[i] = list[i].toThreadState()
	}
	return c.JSON(http.StatusOK, out)
}
