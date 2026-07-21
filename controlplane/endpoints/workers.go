// Hand-written worker endpoints (worker↔control-plane protocol). Routes are
// generated into workers_gen.go (endpoints marked custom in endpoints.yaml);
// bodies live here. Source of truth: spec/models/d2 workers block + workers.d2
// + the worker-execution design doc. runs has NO lease_expires_at — fencing is
// lease_epoch only.
package endpoints

import (
	"context"
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

// WorkersStreamEvents is the single worker→server state channel.
// POST /workers/{id}/runs/{rid}/events. run.started establishes the lease and
// returns the epoch; all other events are fenced on lease_epoch. Source: design
// doc "events endpoint". runs has no lease_expires_at — fence on epoch only.
func (s *Server) WorkersStreamEvents(c echo.Context) error {
	ctx := c.Request().Context()
	wid := c.Param("id")
	rid := c.Param("rid")
	var req WorkerEventsRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	var startedEpoch *int
	for _, ev := range req.Events {
		switch ev.Type {
		case "run.started":
			epoch, err := s.leaseRun(ctx, rid, wid)
			if err != nil {
				return err // already-typed echo error (409/500)
			}
			startedEpoch = &epoch
		case "run.completed", "run.failed":
			if err := s.terminalRun(ctx, rid, ev); err != nil {
				return err
			}
		case "execution.node_started", "execution.node_completed", "execution.node_failed":
			if err := s.nodeEvent(ctx, rid, ev); err != nil {
				return err
			}
		default:
			return echo.NewHTTPError(http.StatusBadRequest, "unknown event type: "+ev.Type)
		}
	}
	if startedEpoch != nil {
		return c.JSON(http.StatusOK, RunStartedResponse{LeaseEpoch: *startedEpoch})
	}
	return c.NoContent(http.StatusOK)
}

// leaseRun re-leases a non-terminal run to wid, bumping lease_epoch, and emits
// run.started. Returns the new epoch. 409 if the run is already terminal.
func (s *Server) leaseRun(ctx context.Context, rid, wid string) (int, error) {
	var epoch int
	err := s.writeTx(ctx, s.Tenant, []Event{{AggregateType: "Run", AggregateID: mustParseUUID(rid), EventType: "run.started"}},
		func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `
				UPDATE runs SET status='in_progress', worker_id=$2,
				  lease_epoch=lease_epoch+1, started_at=COALESCE(started_at, now())
				WHERE id=$1 AND status IN ('queued','in_progress')
				RETURNING lease_epoch`, rid, wid).Scan(&epoch)
		})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, echo.NewHTTPError(http.StatusConflict, "run is terminal or not found")
		}
		return 0, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return epoch, nil
}

// errStaleLease is returned from a projection when the epoch guard rejects the
// write (0 rows affected). writeTxOrHTTP maps it to 409. Because writeTx appends
// the event BEFORE the projection runs, returning this rolls the whole tx back —
// so a stale worker's event is never committed.
var errStaleLease = errors.New("stale lease_epoch")

// nodeEvent records a node execution row + emits the execution.node_* event.
// The epoch guard is ATOMIC (conditional INSERT), not a separate read.
func (s *Server) nodeEvent(ctx context.Context, rid string, ev WorkerEvent) error {
	return s.writeTxOrHTTP(ctx, rid, ev.Type, ev, func(tx pgx.Tx) error {
		ct, err := tx.Exec(ctx, `
			INSERT INTO execution_history (run_id, node_id, node_type, status, input, output, error, duration_ms, completed_at)
			SELECT $1,$2,$3,$4::varchar,$5,$6,$7,$8, CASE WHEN $4::varchar <> 'started' THEN now() END
			WHERE EXISTS (SELECT 1 FROM runs WHERE id=$1 AND lease_epoch=$9)`,
			rid, ev.NodeID, ev.NodeType, ev.NodeStatus,
			jsonOrEmpty(ev.Input), jsonOrEmpty(ev.Output), ev.Error, ev.DurationMs, ev.LeaseEpoch)
		if err != nil {
			return err
		}
		if ct.RowsAffected() == 0 {
			return errStaleLease
		}
		return nil
	})
}

// terminalRun marks the run completed/failed + emits the event. Epoch guarded
// IN the UPDATE (WHERE lease_epoch=$epoch); 0 rows → stale → 409.
func (s *Server) terminalRun(ctx context.Context, rid string, ev WorkerEvent) error {
	status := "completed"
	if ev.Type == "run.failed" {
		status = "failed"
	}
	return s.writeTxOrHTTP(ctx, rid, ev.Type, ev, func(tx pgx.Tx) error {
		ct, err := tx.Exec(ctx,
			`UPDATE runs SET status=$2, completed_at=now(), error=$3 WHERE id=$1 AND lease_epoch=$4`,
			rid, status, ev.Error, ev.LeaseEpoch)
		if err != nil {
			return err
		}
		if ct.RowsAffected() == 0 {
			return errStaleLease
		}
		return nil
	})
}

// writeTxOrHTTP runs a projection inside writeTx (emitting one event) and maps
// errStaleLease → 409, other errors → 500.
func (s *Server) writeTxOrHTTP(ctx context.Context, rid, eventType string, ev WorkerEvent, proj func(pgx.Tx) error) error {
	events := []Event{{AggregateType: "Run", AggregateID: mustParseUUID(rid), EventType: eventType, Payload: mustJSON(ev)}}
	if err := s.writeTx(ctx, s.Tenant, events, proj); err != nil {
		if errors.Is(err, errStaleLease) {
			return echo.NewHTTPError(http.StatusConflict, "stale lease_epoch")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return nil
}
