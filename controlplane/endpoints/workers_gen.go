// Code generated from controlplane/gen/endpoints.yaml — DO NOT EDIT.
// Source of truth: spec/models/d2/endpoint-queries.d2 (via endpoints.yaml).
// Regenerate: go run ./controlplane/gen
package endpoints

import (
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

// WorkersClaim — POST /workers/{id}/runs/claim  (kind: write) — hand-written in workers.go

// WorkersStreamEvents — POST /workers/{id}/runs/{rid}/events  (kind: write) — hand-written in workers.go

// WorkersWriteCheckpoint — POST /threads/{tid}/checkpoints  (kind: write) — hand-written in workers.go

// WorkersReadCheckpoint — GET /threads/{tid}/checkpoints/{ckpt}  (kind: read) — hand-written in workers.go

// WorkersLatestCheckpoint — GET /threads/{tid}/checkpoints/latest  (kind: read) — hand-written in workers.go

// WorkersLoadGraph — GET /workers/runs/{rid}/graph  (kind: read) — hand-written in workers.go
