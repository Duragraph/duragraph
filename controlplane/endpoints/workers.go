// Hand-written worker endpoints (worker↔control-plane protocol). Routes are
// generated into workers_gen.go (endpoints marked custom in endpoints.yaml);
// bodies live here. Source of truth: spec/models/d2 workers block + workers.d2
// + the worker-execution design doc. runs has NO lease_expires_at — fencing is
// lease_epoch only.
package endpoints

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

// WorkersRegister upserts a worker as online. POST /workers/register -> 200.
func (s *Server) WorkersRegister(c echo.Context) error {
	ctx := c.Request().Context()
	var req WorkerRegisterRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if _, err := s.Tenant.Exec(ctx, `
		INSERT INTO workers (worker_id, graphs, capacity, status, lease_expires_at, last_heartbeat_at)
		VALUES ($1, $2, $3, 'online', now() + interval '60 seconds', now())
		ON CONFLICT (worker_id) DO UPDATE
		  SET graphs=EXCLUDED.graphs, capacity=EXCLUDED.capacity, status='online',
		      lease_expires_at=now() + interval '60 seconds', last_heartbeat_at=now()`,
		req.WorkerID, req.Graphs, req.Capacity); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, WorkerRegisterResponse{WorkerID: req.WorkerID, Status: "online"})
}

// WorkersHeartbeat renews the worker lease. POST /workers/{id}/heartbeat -> 200.
func (s *Server) WorkersHeartbeat(c echo.Context) error {
	ctx := c.Request().Context()
	wid := c.Param("id")
	var req WorkerHeartbeatRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	ct, err := s.Tenant.Exec(ctx, `
		UPDATE workers SET status=$2, active_runs=$3,
		  lease_expires_at=now() + interval '60 seconds', last_heartbeat_at=now()
		WHERE worker_id=$1 AND lease_expires_at > now()`,
		wid, req.Status, req.ActiveRuns)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if ct.RowsAffected() == 0 {
		// Unknown worker or expired lease — worker must re-register.
		return echo.NewHTTPError(http.StatusConflict, "worker lease expired or unknown; re-register")
	}
	return c.JSON(http.StatusOK, WorkerHeartbeatResponse{Commands: []string{}})
}

// WorkersDeregister marks the worker offline and requeues its in-flight runs.
// POST /workers/{id}/deregister -> 204.
func (s *Server) WorkersDeregister(c echo.Context) error {
	ctx := c.Request().Context()
	wid := c.Param("id")
	tx, err := s.Tenant.Begin(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `UPDATE workers SET status='offline' WHERE worker_id=$1`, wid); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if _, err := tx.Exec(ctx, `
		UPDATE runs SET status='queued', worker_id=NULL
		WHERE worker_id=$1 AND status='in_progress'`, wid); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if err := tx.Commit(ctx); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

var _ = errors.Is // retained for Tasks 4–5 handlers added to this file
var _ = pgx.ErrNoRows
