// Hand-written worker run-claim endpoint (the PULL half of the worker↔control-
// plane protocol). Route is generated into workers_gen.go; the body lives here
// rather than in workers.go because the claim is the one worker endpoint that
// is a scheduler: it decides WHICH runs a worker gets, under concurrency, and
// that decision is most of the file. Source of truth: endpoints.yaml workers
// group, endpoint `claim` + the worker-execution design doc.
package endpoints

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"

	"github.com/duragraph/duragraph/controlplane/eventstore"
)

// WorkerClaimRequest is the claim body. Limit is accepted as an alias for
// MaxRuns because the spec step says `LIMIT :max_runs` while every other
// paginated surface in this API says `limit`; accepting both costs one line
// and spares a worker author a silent zero-claim.
//
// The body is optional: an absent/empty body claims defaultClaimBatch run.
type WorkerClaimRequest struct {
	MaxRuns int `json:"max_runs"`
	Limit   int `json:"limit"`
}

// Claim batch bounds. A worker that asks for nothing gets one run (the safe
// default: it can always claim again). maxClaimBatch caps what a single call
// can lock at once — the claim holds row locks for the length of its
// transaction, so an unbounded :max_runs would let one worker stall every
// other claimer while it drains the queue.
const (
	defaultClaimBatch = 1
	maxClaimBatch     = 100
)

func (r WorkerClaimRequest) batchSize() int {
	n := r.MaxRuns
	if n <= 0 {
		n = r.Limit
	}
	if n <= 0 {
		return defaultClaimBatch
	}
	if n > maxClaimBatch {
		return maxClaimBatch
	}
	return n
}

// ClaimedRun is one run handed to a worker, matching the spec's
// "returns: run + checkpoint_id (null if fresh, set if resuming)".
//
// LeaseEpoch is carried alongside because the claim IS the lease acquisition:
// every subsequent worker→server event is fenced on this epoch
// (WorkersStreamEvents), so a claim that withheld it would force the worker to
// post a second run.started just to learn its own epoch — which would bump the
// epoch again and fence writes against a token it had never used.
//
// Input is included so a claiming worker can execute without a second round
// trip; the graph itself is still fetched from /workers/runs/{rid}/graph.
type ClaimedRun struct {
	Run          Run             `json:"run"`
	LeaseEpoch   int             `json:"lease_epoch"`
	CheckpointID *int64          `json:"checkpoint_id"`
	GraphID      string          `json:"graph_id,omitempty"`
	Input        json.RawMessage `json:"input,omitempty"`
}

// WorkerClaimResponse wraps the claimed set. Runs is never null — an empty
// queue is `{"runs":[]}`, so a polling worker can range over it unconditionally.
type WorkerClaimResponse struct {
	Runs []ClaimedRun `json:"runs"`
}

// claimedRunRow is runRow plus the per-run latest checkpoint id. Embedded (not
// copied) so the claim scans the exact same run shape every other run handler
// does — one runRow definition, one toAPI mapper.
//
// CheckpointID is `db:"-"`: it is not a runs column and is filled by
// attachLatestCheckpoints after the lease, so RowToStructByName (strict — it
// errors on a field with no matching column) must be told to skip it.
//
// EffectiveGraphID is the graph the run actually matched on — runs.graph_id
// when set, else the assistant's. Reporting runRow.GraphID instead would hand
// the worker an empty string for every run created through the current API
// path, which does not populate the denormalized column.
type claimedRunRow struct {
	runRow
	CheckpointID     *int64  `db:"-"`
	EffectiveGraphID *string `db:"effective_graph_id"`
}

// claimSQL atomically selects and leases the next runs for a worker.
//
// Two things in here are load-bearing:
//
//  1. FOR UPDATE SKIP LOCKED. Workers poll concurrently; without SKIP LOCKED
//     the candidate SELECTs would either serialize (each waiting on the other's
//     row locks, turning the pool into a queue of one) or — with a plain
//     SELECT then UPDATE — hand the SAME run to two workers, which is a
//     double-execution bug no fencing token can undo after the fact. SKIP
//     LOCKED makes concurrent claimers step over each other's in-flight
//     candidates instead.
//
//  2. The select and the update are ONE statement (CTE + UPDATE ... FROM). The
//     row locks taken by the CTE are held for the rest of the transaction, so
//     nothing can slip between "chosen" and "leased".
//
// The graph filter is `effective graph_id = ANY(worker.graphs)`, where the
// effective graph is COALESCE(runs.graph_id, assistants.graph_id): runs.graph_id
// is a denormalization added by migration 005 that the run-create path does not
// populate yet (see runs_create.go — it inserts no graph_id), so filtering on
// the column alone would match nothing and every worker would idle forever.
// Falling back to the run's assistant's graph_id honours the spec's intent
// ("graph_id IN (worker graphs)") against the schema as it actually is. A run
// with no effective graph is deliberately unclaimable: NULL is in no set, and
// handing a worker a run whose graph it never advertised is exactly what this
// filter exists to prevent.
//
// ORDER BY priority DESC, created_at — priority first, then FIFO within a
// priority, per the spec and the idx_runs_claim index that backs it.
const claimSQL = `
WITH candidate AS (
    SELECT r.id
    FROM runs r
    WHERE r.status = 'queued'
      AND COALESCE(r.graph_id, (SELECT a.graph_id FROM assistants a WHERE a.id = r.assistant_id)) = ANY($2::text[])
    ORDER BY r.priority DESC, r.created_at
    LIMIT $3
    FOR UPDATE SKIP LOCKED
)
UPDATE runs r
SET status      = 'in_progress',
    worker_id   = $1,
    lease_epoch = r.lease_epoch + 1,
    started_at  = COALESCE(r.started_at, now()),
    version     = r.version + 1
FROM candidate c
WHERE r.id = c.id
RETURNING r.id, r.thread_id, r.assistant_id, r.status, r.input, r.output, r.error,
          r.metadata, r.kwargs, r.multitask_strategy, r.version, r.lease_epoch,
          r.worker_id, r.priority, r.graph_id, r.created_at, r.started_at,
          r.completed_at, r.updated_at,
          COALESCE(r.graph_id, (SELECT a.graph_id FROM assistants a WHERE a.id = r.assistant_id)) AS effective_graph_id`

// WorkersClaim leases the next queued runs matching the worker's graphs.
// POST /workers/{id}/runs/claim -> 200 WorkerClaimResponse / 409 / 422.
//
// lease_epoch+1 on every claim is the fencing token, and incrementing it here
// is what makes a stolen run safe: the moment this row is re-leased, the
// PREVIOUS holder's epoch is stale, so its node/terminal/checkpoint writes are
// rejected by the epoch guards in workers.go (nodeEvent, terminalRun,
// requiresActionRun, WorkersWriteCheckpoint) instead of corrupting a run
// another worker now owns. Same increment, same meaning as leaseRun — this
// endpoint just reaches the lease by claiming rather than by being told.
//
// started_at uses COALESCE(started_at, now()) rather than the spec's bare
// now(): a run that was requeued (WorkersDeregister) and re-claimed keeps its
// FIRST start, so queue→start latency stays measurable. version+1 IS taken
// literally from the spec here, even though leaseRun does not bump it —
// noted in the report as a divergence between the two lease paths.
func (s *Server) WorkersClaim(c echo.Context) error {
	ctx := c.Request().Context()
	wid, err := pathUUIDString(c, "id")
	if err != nil {
		return err
	}
	var req WorkerClaimRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	// The worker's advertised graphs, and — free with it — proof the worker
	// exists. 409 rather than 404 mirrors WorkersHeartbeat: an unregistered
	// worker is not a missing URL, it is a worker that must re-register. It
	// also has to be caught here, because runs.worker_id FKs workers(worker_id)
	// and the UPDATE below would otherwise fail as an opaque 500.
	var graphs []string
	if err := s.Tenant.QueryRow(ctx, `SELECT graphs FROM workers WHERE worker_id = $1`, wid).Scan(&graphs); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusConflict, "unknown worker; re-register")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if len(graphs) == 0 {
		// A worker advertising no graphs can match no run. Answer without
		// opening a transaction (and without firing a pointless outbox notify).
		return c.JSON(http.StatusOK, WorkerClaimResponse{Runs: []ClaimedRun{}})
	}

	var claimed []claimedRunRow
	// One transaction for select+lease+events+outbox+notify, per the spec's
	// step list. The events are appended INSIDE the projection rather than
	// handed to writeTx up front, because which runs were claimed is only
	// known once the SELECT ... FOR UPDATE SKIP LOCKED has run — and it can
	// only run inside this transaction, since that is what holds the locks.
	// Appending in-projection keeps the identical guarantee writeTx exists to
	// provide: a rollback drops the lease, the events, the outbox rows, and
	// the notify together.
	if err := s.writeTx(ctx, s.Tenant, nil, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, claimSQL, wid, graphs, req.batchSize())
		if err != nil {
			return err
		}
		claimed, err = pgx.CollectRows(rows, pgx.RowToStructByName[claimedRunRow])
		if err != nil {
			return err
		}
		if len(claimed) == 0 {
			return nil
		}
		if err := attachLatestCheckpoints(ctx, tx, claimed); err != nil {
			return err
		}
		// One run.started per claimed run, on the RUN's own aggregate — the
		// event stream a run's checkpoints and later events all hang off
		// (WorkersWriteCheckpoint resolves the stream by aggregate_id), so a
		// synthetic aggregate id here would strand every subsequent write.
		events := make([]Event, 0, len(claimed))
		for _, r := range claimed {
			events = append(events, Event{
				AggregateType: "Run",
				AggregateID:   r.ID,
				EventType:     "run.started",
				Payload: mustJSON(map[string]any{
					"run_id":      r.ID,
					"worker_id":   wid,
					"lease_epoch": r.LeaseEpoch,
				}),
			})
		}
		return appendEvents(ctx, tx, events)
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	resp := WorkerClaimResponse{Runs: make([]ClaimedRun, 0, len(claimed))}
	for _, r := range claimed {
		resp.Runs = append(resp.Runs, ClaimedRun{
			Run:          r.toAPI(),
			LeaseEpoch:   r.LeaseEpoch,
			CheckpointID: r.CheckpointID,
			GraphID:      deref(r.EffectiveGraphID),
			Input:        json.RawMessage(r.Input),
		})
	}
	return c.JSON(http.StatusOK, resp)
}

// attachLatestCheckpoints fills CheckpointID for each claimed run: the id of
// its newest snapshot, or nil when the run has never checkpointed (a fresh
// run — the spec's "null if fresh, set if resuming"). It is what tells a
// worker whether to start the graph from its entry node or resume from state.
//
// Newest is max(id), NOT max(version): snapshots.version is per event-stream
// while id is BIGSERIAL, i.e. the true global write order — the same reasoning
// the thread state/history reads already follow (see rows.go DIVERGENCES).
// One grouped query for the whole batch rather than one per run.
func attachLatestCheckpoints(ctx context.Context, tx pgx.Tx, claimed []claimedRunRow) error {
	ids := make([]uuid.UUID, 0, len(claimed))
	for _, r := range claimed {
		ids = append(ids, r.ID)
	}
	rows, err := tx.Query(ctx, `
		SELECT aggregate_id, max(id) AS checkpoint_id
		FROM snapshots
		WHERE aggregate_type = 'Run' AND aggregate_id = ANY($1::uuid[])
		GROUP BY aggregate_id`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	latest := make(map[uuid.UUID]int64, len(claimed))
	for rows.Next() {
		var id uuid.UUID
		var ckpt int64
		if err := rows.Scan(&id, &ckpt); err != nil {
			return err
		}
		latest[id] = ckpt
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for i := range claimed {
		if ckpt, ok := latest[claimed[i].ID]; ok {
			c := ckpt
			claimed[i].CheckpointID = &c
		}
	}
	return nil
}

// appendEvents is writeTx's own append loop, exposed for the events whose
// aggregate ids are discovered inside the transaction (see WorkersClaim).
func appendEvents(ctx context.Context, tx pgx.Tx, events []Event) error {
	for _, e := range events {
		if err := eventstore.Append(ctx, tx, e); err != nil {
			return err
		}
	}
	return nil
}
