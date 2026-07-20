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

// AdminListUsers — GET /api/admin/users  (kind: read)
//   - SELECT * FROM users WHERE status = :filter ORDER BY created_at LIMIT :limit OFFSET :offset
//   - SELECT count(*) FROM users WHERE status = :filter
func (s *Server) AdminListUsers(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	// TODO read query (no side effects):
	//   SELECT * FROM users WHERE status = :filter ORDER BY created_at LIMIT :limit OFFSET :offset
	//   SELECT count(*) FROM users WHERE status = :filter
	return c.JSON(http.StatusOK, map[string]any{})
}

// AdminApprove — POST /api/admin/users/{id}/approve  (kind: write)
//   - SELECT users WHERE id=:id AND status='pending' (validate)
//   - UPDATE users SET status='approved'
//   - UPDATE tenants SET status='provisioning' WHERE user_id=:id
//   - INSERT events: user.approved + tenant.provisioning
//   - INSERT outbox (both events, same TX)
//   - pg_notify('outbox_new',”)
func (s *Server) AdminApprove(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	var req map[string]any // TODO: bind OpenAPI type (AdminApprove request schema)
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	aggID := uuid.New() // TODO: new id for create; parse from path param for update/cancel/etc.
	events := []Event{
		{AggregateType: "User", AggregateID: aggID, EventType: "user.approved"},
		{AggregateType: "Tenant", AggregateID: aggID, EventType: "tenant.provisioning"},
	}
	if err := s.writeTx(ctx, s.Platform, events, func(tx pgx.Tx) error {
		// TODO projection write:
		//   SELECT users WHERE id=:id AND status='pending' (validate)
		//   UPDATE users SET status='approved'
		//   UPDATE tenants SET status='provisioning' WHERE user_id=:id
		//   INSERT events: user.approved + tenant.provisioning
		//   INSERT outbox (both events, same TX)
		//   pg_notify('outbox_new','')
		return nil
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{}) // TODO: return OpenAPI response type
}

// AdminReject — POST /api/admin/users/{id}/reject  (kind: write)
//   - SELECT users WHERE id=:id AND status='pending'
//   - UPDATE users SET status='suspended'
//   - INSERT events: user.rejected
//   - INSERT outbox (same TX)
//   - pg_notify('outbox_new',”)
func (s *Server) AdminReject(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	var req map[string]any // TODO: bind OpenAPI type (AdminReject request schema)
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	aggID := uuid.New() // TODO: new id for create; parse from path param for update/cancel/etc.
	events := []Event{
		{AggregateType: "User", AggregateID: aggID, EventType: "user.rejected"},
	}
	if err := s.writeTx(ctx, s.Platform, events, func(tx pgx.Tx) error {
		// TODO projection write:
		//   SELECT users WHERE id=:id AND status='pending'
		//   UPDATE users SET status='suspended'
		//   INSERT events: user.rejected
		//   INSERT outbox (same TX)
		//   pg_notify('outbox_new','')
		return nil
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{}) // TODO: return OpenAPI response type
}

// AdminSuspend — POST /api/admin/users/{id}/suspend  (kind: write)
//   - SELECT users WHERE id=:id AND status='approved'
//   - UPDATE users SET status='suspended'
//   - UPDATE tenants SET status='suspended' WHERE user_id=:id
//   - INSERT events: user.suspended + tenant.suspended
//   - INSERT outbox (same TX)
//   - pg_notify('outbox_new',”)
func (s *Server) AdminSuspend(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	var req map[string]any // TODO: bind OpenAPI type (AdminSuspend request schema)
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	aggID := uuid.New() // TODO: new id for create; parse from path param for update/cancel/etc.
	events := []Event{
		{AggregateType: "User", AggregateID: aggID, EventType: "user.suspended"},
		{AggregateType: "Tenant", AggregateID: aggID, EventType: "tenant.suspended"},
	}
	if err := s.writeTx(ctx, s.Platform, events, func(tx pgx.Tx) error {
		// TODO projection write:
		//   SELECT users WHERE id=:id AND status='approved'
		//   UPDATE users SET status='suspended'
		//   UPDATE tenants SET status='suspended' WHERE user_id=:id
		//   INSERT events: user.suspended + tenant.suspended
		//   INSERT outbox (same TX)
		//   pg_notify('outbox_new','')
		return nil
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{}) // TODO: return OpenAPI response type
}

// AdminResume — POST /api/admin/users/{id}/resume  (kind: write)
//   - SELECT users WHERE id=:id AND status='suspended'
//   - UPDATE users SET status='approved'
//   - UPDATE tenants SET status='approved' WHERE user_id=:id
//   - # optional user.resumed event — not yet in spec
func (s *Server) AdminResume(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	var req map[string]any // TODO: bind OpenAPI type (AdminResume request schema)
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	// projection-only write (not event-sourced — no outbox):
	//   SELECT users WHERE id=:id AND status='suspended'
	//   UPDATE users SET status='approved'
	//   UPDATE tenants SET status='approved' WHERE user_id=:id
	//   # optional user.resumed event — not yet in spec
	return c.JSON(http.StatusOK, map[string]any{}) // TODO: return OpenAPI response type
}

// AdminRetryMigration — POST /api/admin/tenants/{id}/retry-migration  (kind: write)
//   - SELECT tenants WHERE id=:id AND status='provisioning_failed'
//   - UPDATE tenants SET status='provisioning', failure_reason=NULL
//   - INSERT events: tenant.provisioning
//   - INSERT outbox (same TX)
//   - pg_notify('outbox_new',”)
func (s *Server) AdminRetryMigration(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	var req map[string]any // TODO: bind OpenAPI type (AdminRetryMigration request schema)
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	aggID := uuid.New() // TODO: new id for create; parse from path param for update/cancel/etc.
	events := []Event{
		{AggregateType: "Tenant", AggregateID: aggID, EventType: "tenant.provisioning"},
	}
	if err := s.writeTx(ctx, s.Platform, events, func(tx pgx.Tx) error {
		// TODO projection write:
		//   SELECT tenants WHERE id=:id AND status='provisioning_failed'
		//   UPDATE tenants SET status='provisioning', failure_reason=NULL
		//   INSERT events: tenant.provisioning
		//   INSERT outbox (same TX)
		//   pg_notify('outbox_new','')
		return nil
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{}) // TODO: return OpenAPI response type
}

// AdminMetrics — GET /api/admin/metrics  (kind: read)
//   - Query Mimir PromQL: sum by (tenant_id) rate(runs_total[window])
func (s *Server) AdminMetrics(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	// TODO read query (no side effects):
	//   Query Mimir PromQL: sum by (tenant_id) rate(runs_total[window])
	return c.JSON(http.StatusOK, map[string]any{})
}

// AdminMetricsTenant — GET /api/admin/metrics/{tenant_id}  (kind: read)
//   - Query Mimir PromQL: rate(runs_total{tenant_id=...}[window])
func (s *Server) AdminMetricsTenant(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	// TODO read query (no side effects):
	//   Query Mimir PromQL: rate(runs_total{tenant_id=...}[window])
	return c.JSON(http.StatusOK, map[string]any{})
}
