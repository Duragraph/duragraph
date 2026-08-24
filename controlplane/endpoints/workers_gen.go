// Code generated from controlplane/gen/endpoints.yaml — DO NOT EDIT.
// Source of truth: spec/models/d2/endpoint-queries.d2 (via endpoints.yaml).
// Regenerate: go run ./controlplane/gen
package endpoints

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

// RegisterWorkers mounts the workers endpoints on g (the /api/v1 group).
func (s *Server) RegisterWorkers(g *echo.Group) {
	g.POST("/workers/register", s.WorkersRegister)
	g.POST("/workers/:id/heartbeat", s.WorkersHeartbeat)
	g.POST("/workers/:id/deregister", s.WorkersDeregister)
	g.POST("/workers/:id/runs/claim", s.WorkersClaim)
	g.POST("/workers/:id/runs/:rid/events", s.WorkersStreamEvents)
	g.POST("/threads/:tid/checkpoints", s.WorkersWriteCheckpoint)
	g.GET("/threads/:tid/checkpoints/:ckpt", s.WorkersReadCheckpoint)
	g.GET("/threads/:tid/checkpoints/latest", s.WorkersLatestCheckpoint)
	g.GET("/workers/runs/:rid/graph", s.WorkersLoadGraph)
}

// WorkersRegister — POST /workers/register  (kind: write) — hand-written in workers.go

// WorkersHeartbeat — POST /workers/{id}/heartbeat  (kind: write) — hand-written in workers.go

// WorkersDeregister — POST /workers/{id}/deregister  (kind: write) — hand-written in workers.go

// WorkersClaim — POST /workers/{id}/runs/claim  (kind: write)
//   - SELECT runs WHERE status='queued' AND graph_id IN (worker graphs) ORDER BY priority DESC, created_at FOR UPDATE SKIP LOCKED LIMIT :max_runs
//   - SELECT snapshots: latest checkpoint_id per run (if resuming)
//   - UPDATE runs SET status='in_progress', worker_id=:id, lease_epoch=lease_epoch+1, started_at=now(), version=version+1
//   - INSERT events: event_type='run.started' for each claimed run
//   - INSERT outbox (same TX)
//   - pg_notify('outbox_new',”)
func (s *Server) WorkersClaim(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	var req map[string]any // TODO: bind OpenAPI type (WorkersClaim request schema)
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	aggID := uuid.New() // TODO: new id for create; parse from path param for update/cancel/etc.
	events := []Event{
		{AggregateType: "Run", AggregateID: aggID, EventType: "run.started"},
	}
	if err := s.writeTx(ctx, s.Tenant, events, func(tx pgx.Tx) error {
		// TODO projection write:
		//   SELECT runs WHERE status='queued' AND graph_id IN (worker graphs) ORDER BY priority DESC, created_at FOR UPDATE SKIP LOCKED LIMIT :max_runs
		//   SELECT snapshots: latest checkpoint_id per run (if resuming)
		//   UPDATE runs SET status='in_progress', worker_id=:id, lease_epoch=lease_epoch+1, started_at=now(), version=version+1
		//   INSERT events: event_type='run.started' for each claimed run
		//   INSERT outbox (same TX)
		//   pg_notify('outbox_new','')
		return nil
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{}) // TODO: return OpenAPI response type
}

// WorkersStreamEvents — POST /workers/{id}/runs/{rid}/events  (kind: write) — hand-written in workers.go

// WorkersWriteCheckpoint — POST /threads/{tid}/checkpoints  (kind: write) — hand-written in workers.go

// WorkersReadCheckpoint — GET /threads/{tid}/checkpoints/{ckpt}  (kind: read) — hand-written in workers.go

// WorkersLatestCheckpoint — GET /threads/{tid}/checkpoints/latest  (kind: read) — hand-written in workers.go

// WorkersLoadGraph — GET /workers/runs/{rid}/graph  (kind: read) — hand-written in workers.go
