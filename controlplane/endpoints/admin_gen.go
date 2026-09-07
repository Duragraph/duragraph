// Code generated from controlplane/gen/endpoints.yaml — DO NOT EDIT.
// Source of truth: spec/models/d2/endpoint-queries.d2 (via endpoints.yaml).
// Regenerate: go run ./controlplane/gen
package endpoints

import (
	"github.com/labstack/echo/v4"
)

// RegisterAdmin mounts the admin endpoints on g (the /api/v1 group).
func (s *Server) RegisterAdmin(g *echo.Group) {
	g.GET("/api/admin/users", s.AdminListUsers)
	g.POST("/api/admin/users/:id/approve", s.AdminApprove)
	g.POST("/api/admin/users/:id/reject", s.AdminReject)
	g.POST("/api/admin/users/:id/suspend", s.AdminSuspend)
	g.POST("/api/admin/users/:id/resume", s.AdminResume)
	g.POST("/api/admin/tenants/:id/retry-migration", s.AdminRetryMigration)
	g.GET("/api/admin/metrics", s.AdminMetrics)
	g.GET("/api/admin/metrics/:tenant_id", s.AdminMetricsTenant)
}

// AdminListUsers — GET /api/admin/users  (kind: read) — hand-written in admin.go

// AdminApprove — POST /api/admin/users/{id}/approve  (kind: write) — hand-written in admin.go

// AdminReject — POST /api/admin/users/{id}/reject  (kind: write) — hand-written in admin.go

// AdminSuspend — POST /api/admin/users/{id}/suspend  (kind: write) — hand-written in admin.go

// AdminResume — POST /api/admin/users/{id}/resume  (kind: write) — hand-written in admin.go

// AdminRetryMigration — POST /api/admin/tenants/{id}/retry-migration  (kind: write) — hand-written in admin.go

// AdminMetrics — GET /api/admin/metrics  (kind: read) — hand-written in admin.go

// AdminMetricsTenant — GET /api/admin/metrics/{tenant_id}  (kind: read) — hand-written in admin.go
