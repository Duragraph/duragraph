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
		case "run.requires_action":
			if err := s.requiresActionRun(ctx, rid, ev); err != nil {
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

// nodeEvent records a node execution + emits the execution.node_* event.
//
// execution_history is "one row per node execution" (003_run.up.sql), and its
// columns say the same: started_at NOT NULL, completed_at and duration_ms
// nullable. So a node's lifecycle TRANSITIONS one row rather than appending
// several — execution.node_started INSERTs it, and node_completed/node_failed
// UPDATE that row in place. Appending on every event would give two rows per
// node and turn "how many times did N run" into a question about event counts.
//
// The completing UPDATE targets the newest still-'started' row for (run_id,
// node_id), so a node that runs more than once (a cycle) closes its own
// attempt rather than an earlier one. If no started row exists the write falls
// back to an INSERT: a worker that reports only completion — as every worker
// did before node_started was emitted — still records its execution.
//
// The epoch guard is ATOMIC in both paths (a conditional INSERT, and a join to
// runs in the UPDATE), never a separate read.
func (s *Server) nodeEvent(ctx context.Context, rid string, ev WorkerEvent) error {
	return s.writeTxOrHTTP(ctx, rid, ev.Type, ev, func(tx pgx.Tx) error {
		if ev.NodeStatus == "started" {
			return insertNodeExecution(ctx, tx, rid, ev)
		}
		ct, err := tx.Exec(ctx, `
			UPDATE execution_history h
			SET status = $3::varchar, output = $4, error = $5,
			    duration_ms = COALESCE($6, (EXTRACT(EPOCH FROM (now() - h.started_at)) * 1000)::integer),
			    completed_at = now()
			WHERE h.id = (
			    SELECT id FROM execution_history
			    WHERE run_id = $1 AND node_id = $2 AND status = 'started'
			    ORDER BY id DESC LIMIT 1)
			  AND EXISTS (SELECT 1 FROM runs WHERE id = $1 AND lease_epoch = $7)`,
			rid, ev.NodeID, ev.NodeStatus,
			jsonOrEmpty(ev.Output), ev.Error, ev.DurationMs, ev.LeaseEpoch)
		if err != nil {
			return err
		}
		if ct.RowsAffected() > 0 {
			return nil
		}
		// No open attempt to close. Either the run is fenced, or the worker
		// never announced a start — insert so the execution is still recorded,
		// and let the epoch guard inside decide which of the two it was.
		return insertNodeExecution(ctx, tx, rid, ev)
	})
}

// insertNodeExecution writes a new execution_history row, epoch-fenced.
// completed_at is set for any terminal status so a fallback insert (a worker
// reporting only completion) is not left looking perpetually in-flight.
func insertNodeExecution(ctx context.Context, tx pgx.Tx, rid string, ev WorkerEvent) error {
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

// requiresActionRun suspends a run for human-in-the-loop: it flips
// runs.status → 'requires_action' (epoch-fenced) and records the interrupt row,
// emitting run.requires_action. Both writes share the writeTx transaction, so a
// stale worker (fenced by the UPDATE) never leaves an orphan interrupt.
//
// Idempotent under JetStream redelivery: the interrupt INSERT is guarded by NOT
// EXISTS on an unresolved (run_id, node_id), so re-posting the same pause does
// not create a duplicate interrupt row. The status UPDATE tolerates a run
// already in 'requires_action' (WHERE status IN ('in_progress','requires_action'))
// so a redelivered pause is not fenced as stale.
func (s *Server) requiresActionRun(ctx context.Context, rid string, ev WorkerEvent) error {
	reason := ev.Reason
	if reason == "" {
		reason = "approval_required"
	}
	return s.writeTxOrHTTP(ctx, rid, ev.Type, ev, func(tx pgx.Tx) error {
		ct, err := tx.Exec(ctx, `
			UPDATE runs SET status='requires_action'
			WHERE id=$1 AND lease_epoch=$2
			  AND status IN ('in_progress','requires_action')`,
			rid, ev.LeaseEpoch)
		if err != nil {
			return err
		}
		if ct.RowsAffected() == 0 {
			return errStaleLease
		}
		// Casts are required: in an INSERT..SELECT the select-list params carry no
		// column context, so pgx cannot infer their types — and $5 is passed as a
		// nil []byte (NULL tool_calls), which is doubly ambiguous. Explicit casts
		// pin every parameter and avoid SQLSTATE 42P08.
		if _, err := tx.Exec(ctx, `
			INSERT INTO interrupts (run_id, node_id, reason, state, tool_calls)
			SELECT $1::uuid, $2::text, $3::text, $4::jsonb, $5::jsonb
			WHERE NOT EXISTS (
			  SELECT 1 FROM interrupts WHERE run_id=$1::uuid AND node_id=$2::text AND NOT resolved)`,
			rid, ev.NodeID, reason, jsonOrEmpty(ev.State), nullableJSON(ev.ToolCalls)); err != nil {
			return err
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

// WorkersWriteCheckpoint upserts a run snapshot, epoch-fenced.
// POST /threads/{tid}/checkpoints -> 200 {checkpoint_id}.
func (s *Server) WorkersWriteCheckpoint(c echo.Context) error {
	ctx := c.Request().Context()
	var req CheckpointWriteRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	rid := req.RunID.String()
	// Resolve the run's event stream (created by run.started's writeTx).
	var streamID string
	if err := s.Tenant.QueryRow(ctx,
		`SELECT stream_id FROM event_streams WHERE aggregate_type='Run' AND aggregate_id=$1`, rid).Scan(&streamID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusConflict, "run stream not initialized (post run.started first)")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	// Atomic epoch guard: the INSERT..SELECT only fires when the run's current
	// lease_epoch matches; a stale worker inserts nothing → 0 rows → 409.
	var id int64
	err := s.Tenant.QueryRow(ctx, `
		INSERT INTO snapshots (stream_id, aggregate_type, aggregate_id, version, state)
		SELECT $1, 'Run', $2, $3, $4
		WHERE EXISTS (SELECT 1 FROM runs WHERE id=$2 AND lease_epoch=$5)
		ON CONFLICT (stream_id, version) DO UPDATE SET state=EXCLUDED.state
		RETURNING id`,
		streamID, rid, req.Version, jsonOrEmpty(req.State), req.LeaseEpoch).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusConflict, "stale lease_epoch")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, CheckpointWriteResponse{CheckpointID: id})
}

// WorkersReadCheckpoint returns one snapshot by id, scoped to the thread's runs.
// GET /threads/{tid}/checkpoints/{ckpt} -> 200 / 404.
func (s *Server) WorkersReadCheckpoint(c echo.Context) error {
	ctx := c.Request().Context()
	tid := c.Param("tid")
	ckpt := c.Param("ckpt")
	rows, err := s.Tenant.Query(ctx, `
		SELECT id, stream_id, aggregate_id, version, state, created_at
		FROM snapshots
		WHERE id=$1 AND aggregate_id IN (SELECT id FROM runs WHERE thread_id=$2)`, ckpt, tid)
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
	return c.JSON(http.StatusOK, row.toAPI())
}

// WorkersLatestCheckpoint returns the highest-version snapshot for a run, for
// worker resume. GET /threads/{tid}/checkpoints/latest?run_id=... -> 200 / 404.
func (s *Server) WorkersLatestCheckpoint(c echo.Context) error {
	ctx := c.Request().Context()
	rid := c.QueryParam("run_id")
	if rid == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "run_id is required")
	}
	rows, err := s.Tenant.Query(ctx, `
		SELECT id, stream_id, aggregate_id, version, state, created_at
		FROM snapshots WHERE aggregate_id=$1 ORDER BY version DESC LIMIT 1`, rid)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	row, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[snapshotRow])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "no checkpoint")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, row.toAPI())
}

// WorkersLoadGraph returns the GraphDefinition a worker must execute for a run:
// the latest graph registered for the run's assistant. Keyed by run_id so the
// worker passes only the id it already holds (staying decoupled from the DB
// schema). GET /workers/{id}/runs is claim; this is /workers/runs/{rid}/graph
// -> 200 WorkerGraphResponse / 404 (unknown run, or assistant with no graph).
func (s *Server) WorkersLoadGraph(c echo.Context) error {
	ctx := c.Request().Context()
	rid := c.Param("rid")
	var resp WorkerGraphResponse
	// Latest graph for the run's assistant. Ordered by created_at, NOT version:
	// version is VARCHAR(50), so `ORDER BY version DESC` sorts lexicographically
	// ('10' < '2') and would pick the wrong graph once an assistant has more than
	// one version. Slice-1 assumes one graph per assistant; created_at keeps
	// "latest wins" correct if that assumption is ever relaxed. (Selecting by
	// graph_id/name is a separate TARGET — see graph-engine.d2 loader.)
	err := s.Tenant.QueryRow(ctx, `
		SELECT nodes, edges, config FROM graphs
		WHERE assistant_id = (SELECT assistant_id FROM runs WHERE id = $1)
		ORDER BY created_at DESC LIMIT 1`, rid).Scan(&resp.Nodes, &resp.Edges, &resp.Config)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "no graph for run")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}
