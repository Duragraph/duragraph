// Package admin implements the platform admin HTTP handlers for
// /api/admin/* endpoints. The contract lives in
// duragraph-spec/api/platform.yaml (Admin section); each Handler
// method maps 1:1 to a path there.
//
// Auth chain: each /api/admin/* request is gated by
// `TenantMiddleware -> AdminAuthMiddleware`. By the time a Handler
// method runs, the request context carries a verified `user_id` and
// `role=admin`. The actor's user_id flows from
// `middleware.UserIDFromCtx(c)` into command handlers as
// ApprovedByUserID / RejectedByUserID / SuspendedByUserID /
// ResumedByUserID. Self-action guards live on the User aggregate
// (see internal/domain/user/user.go § Approve / Reject / Suspend);
// the handler does not duplicate them.
//
// Error mapping (consistent across all action endpoints):
//
//	errors.ErrNotFound       -> 404
//	errors.ErrInvalidState   -> 400
//	errors.ErrInvalidInput   -> 400 (covers self-action guards)
//	anything else            -> 500
//
// 401 is returned defensively when UserIDFromCtx comes back empty —
// the middleware chain should make this unreachable, but checking
// here means a misconfigured route ("AdminAuthMiddleware without
// TenantMiddleware in front") fails closed.
package admin

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/duragraph/duragraph/internal/application/command"
	"github.com/duragraph/duragraph/internal/domain/tenant"
	"github.com/duragraph/duragraph/internal/domain/user"
	"github.com/duragraph/duragraph/internal/infrastructure/http/middleware"
	pkgerrors "github.com/duragraph/duragraph/internal/pkg/errors"
)

// Pagination limits — match duragraph-spec/api/platform.yaml §
// /api/admin/users.parameters (limit min=1, max=200, default=50;
// offset min=0, default=0).
const (
	defaultLimit = 50
	maxLimit     = 200
)

// Handler is the HTTP layer for /api/admin/*. All dependencies are
// required; pass a nil MetricsBackend if Mimir is not configured —
// the metrics endpoints will then return 503 Service Unavailable
// instead of crashing on a nil deref.
type Handler struct {
	userRepo   user.Repository
	tenantRepo tenant.Repository

	approve        *command.ApproveUserHandler
	reject         *command.RejectUserHandler
	suspend        *command.SuspendUserHandler
	resume         *command.ResumeUserHandler
	retryMigrate   *command.RetryTenantMigrationHandler
	metricsBackend MetricsBackend
}

// NewHandler constructs a Handler. metricsBackend may be nil — the
// metrics endpoints then fail closed with 503. The user/tenant repos
// and the four user-action command handlers must all be non-nil
// (passing nil yields lazy nil-deref panics on first request, which
// is harder to diagnose than a constructor-time check).
func NewHandler(
	userRepo user.Repository,
	tenantRepo tenant.Repository,
	approve *command.ApproveUserHandler,
	reject *command.RejectUserHandler,
	suspend *command.SuspendUserHandler,
	resume *command.ResumeUserHandler,
	retryMigrate *command.RetryTenantMigrationHandler,
	metricsBackend MetricsBackend,
) *Handler {
	return &Handler{
		userRepo:       userRepo,
		tenantRepo:     tenantRepo,
		approve:        approve,
		reject:         reject,
		suspend:        suspend,
		resume:         resume,
		retryMigrate:   retryMigrate,
		metricsBackend: metricsBackend,
	}
}

// Register mounts the admin routes under the given Echo group. The
// caller is responsible for applying TenantMiddleware +
// AdminAuthMiddleware on the group BEFORE calling Register; the
// handler does not re-apply them.
func (h *Handler) Register(g *echo.Group) {
	g.GET("/users", h.ListUsers)
	g.POST("/users/:user_id/approve", h.ApproveUser)
	g.POST("/users/:user_id/reject", h.RejectUser)
	g.POST("/users/:user_id/suspend", h.SuspendUser)
	g.POST("/users/:user_id/resume", h.ResumeUser)
	g.POST("/tenants/:tenant_id/retry-migration", h.RetryTenantMigration)
	g.GET("/metrics", h.GetMetrics)
	g.GET("/metrics/:tenant_id", h.GetTenantMetrics)
}

// ListUsers handles GET /api/admin/users.
//
// Pagination clamping rather than 400-on-out-of-range: the spec
// declares min/max via OpenAPI but does not specify validator
// behaviour at the engine layer. Clamping is gentler on dashboard
// callers that pre-fill with a stale "limit=500" link — they still
// get a useful response.
func (h *Handler) ListUsers(c echo.Context) error {
	if _, ok := middleware.UserIDFromCtx(c); !ok {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "missing authenticated user",
		})
	}

	ctx := c.Request().Context()

	// status query param — optional. Empty string means "all
	// statuses"; any other value must match the user.Status enum.
	var statusFilter *user.Status
	if raw := c.QueryParam("status"); raw != "" {
		s := user.Status(raw)
		if !s.IsValid() {
			return c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "invalid_request",
				Message: "invalid status filter",
				Details: map[string]any{"status": raw},
			})
		}
		statusFilter = &s
	}

	limit := parseIntDefault(c.QueryParam("limit"), defaultLimit)
	if limit < 1 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	offset := parseIntDefault(c.QueryParam("offset"), 0)
	if offset < 0 {
		offset = 0
	}

	users, err := h.userRepo.List(ctx, statusFilter, limit, offset)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "internal_error",
			Message: "failed to list users",
		})
	}

	total, err := h.userRepo.CountByStatus(ctx, statusFilter)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "internal_error",
			Message: "failed to count users",
		})
	}

	dtos := make([]User, 0, len(users))
	for _, u := range users {
		dtos = append(dtos, h.userToDTO(ctx, u))
	}

	return c.JSON(http.StatusOK, AdminUserListResponse{
		Users:  dtos,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	})
}

// userToDTO maps a domain user to the API DTO. Looks up the tenant
// id when the user is approved (the spec's User.tenant_id is nullable
// for pending users; populated on approval). Failures to fetch the
// tenant are absorbed — the user list endpoint should not 500 because
// one tenant-by-user lookup blew up. The dashboard renders "—" for a
// nil tenant_id.
//
// Trade-off: this is N+1 against tenantRepo (one extra query per
// approved user in the page). Page sizes are bounded at maxLimit=200
// and the platform.tenants table is tiny (one row per tenant — the
// projection table, not per-tenant DB), so this is fine at v0. A
// future optimisation could join in the SQL or add a batch
// `GetByUserIDs` repo method; out of scope here.
func (h *Handler) userToDTO(ctx context.Context, u *user.User) User {
	dto := User{
		ID:            u.ID(),
		Email:         u.Email(),
		OAuthProvider: u.OAuthProvider(),
		Role:          string(u.Role()),
		Status:        string(u.Status()),
		CreatedAt:     u.CreatedAt(),
		UpdatedAt:     u.UpdatedAt(),
	}
	if u.Status() == user.StatusApproved {
		t, err := h.tenantRepo.GetByUserID(ctx, u.ID())
		if err == nil {
			id := t.ID()
			dto.TenantID = &id
		}
	}
	return dto
}

// ApproveUser handles POST /api/admin/users/:user_id/approve.
func (h *Handler) ApproveUser(c echo.Context) error {
	actorID, ok := middleware.UserIDFromCtx(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "unauthorized", Message: "missing authenticated user",
		})
	}
	userID := c.Param("user_id")
	if userID == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "invalid_request", Message: "user_id is required",
		})
	}

	err := h.approve.Handle(c.Request().Context(), command.ApproveUser{
		UserID:           userID,
		ApprovedByUserID: actorID,
	})
	if err != nil {
		return mapDomainError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// RejectUser handles POST /api/admin/users/:user_id/reject. Body is
// optional per the spec — an empty body is fine and `reason` ends up
// "" in the audit log.
func (h *Handler) RejectUser(c echo.Context) error {
	actorID, ok := middleware.UserIDFromCtx(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "unauthorized", Message: "missing authenticated user",
		})
	}
	userID := c.Param("user_id")
	if userID == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "invalid_request", Message: "user_id is required",
		})
	}

	reason := bindOptionalReason(c)

	err := h.reject.Handle(c.Request().Context(), command.RejectUser{
		UserID:           userID,
		RejectedByUserID: actorID,
		Reason:           reason,
	})
	if err != nil {
		return mapDomainError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// SuspendUser handles POST /api/admin/users/:user_id/suspend.
func (h *Handler) SuspendUser(c echo.Context) error {
	actorID, ok := middleware.UserIDFromCtx(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "unauthorized", Message: "missing authenticated user",
		})
	}
	userID := c.Param("user_id")
	if userID == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "invalid_request", Message: "user_id is required",
		})
	}

	reason := bindOptionalReason(c)

	err := h.suspend.Handle(c.Request().Context(), command.SuspendUser{
		UserID:            userID,
		SuspendedByUserID: actorID,
		Reason:            reason,
	})
	if err != nil {
		return mapDomainError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// ResumeUser handles POST /api/admin/users/:user_id/resume.
func (h *Handler) ResumeUser(c echo.Context) error {
	actorID, ok := middleware.UserIDFromCtx(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "unauthorized", Message: "missing authenticated user",
		})
	}
	userID := c.Param("user_id")
	if userID == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "invalid_request", Message: "user_id is required",
		})
	}

	err := h.resume.Handle(c.Request().Context(), command.ResumeUser{
		UserID:          userID,
		ResumedByUserID: actorID,
	})
	if err != nil {
		return mapDomainError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// RetryTenantMigration handles POST
// /api/admin/tenants/:tenant_id/retry-migration.
func (h *Handler) RetryTenantMigration(c echo.Context) error {
	actorID, ok := middleware.UserIDFromCtx(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "unauthorized", Message: "missing authenticated user",
		})
	}
	tenantID := c.Param("tenant_id")
	if tenantID == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "invalid_request", Message: "tenant_id is required",
		})
	}

	err := h.retryMigrate.Handle(c.Request().Context(), command.RetryTenantMigration{
		TenantID:        tenantID,
		RetriedByUserID: actorID,
	})
	if err != nil {
		return mapDomainError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// GetMetrics handles GET /api/admin/metrics. Returns 503 if no
// metrics backend is configured (MIMIR_URL empty in dev). Treating it
// as an *infrastructure* unavailable rather than a 500 makes the
// endpoint diagnosable — the dashboard can render "metrics not
// configured" instead of "internal error".
func (h *Handler) GetMetrics(c echo.Context) error {
	if h.metricsBackend == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			Error:   "metrics_backend_not_configured",
			Message: "metrics backend not configured",
		})
	}
	window := resolveWindow(c.QueryParam("window"))
	tenants, totals, err := fetchAdminMetrics(c.Request().Context(), h.metricsBackend, window, "")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "internal_error",
			Message: "failed to query metrics backend",
		})
	}
	return c.JSON(http.StatusOK, toAdminMetricsResponse(window, tenants, totals))
}

// GetTenantMetrics handles GET /api/admin/metrics/:tenant_id. Same
// shape as GetMetrics filtered to the path tenant_id.
//
// 404 vs 200-with-zeros: the spec calls out 404 for unknown tenants.
// We hit the tenant repo first to surface that case explicitly,
// rather than relying on an empty PromQL result (which could mean
// "tenant exists but had no traffic in the window" — distinct from
// "tenant doesn't exist"). This costs one extra DB roundtrip per
// drilldown — cheap relative to the parallel Mimir queries.
func (h *Handler) GetTenantMetrics(c echo.Context) error {
	if h.metricsBackend == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			Error:   "metrics_backend_not_configured",
			Message: "metrics backend not configured",
		})
	}
	tenantID := c.Param("tenant_id")
	if tenantID == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "invalid_request", Message: "tenant_id is required",
		})
	}

	ctx := c.Request().Context()
	if _, err := h.tenantRepo.GetByID(ctx, tenantID); err != nil {
		if errors.Is(err, pkgerrors.ErrNotFound) {
			return c.JSON(http.StatusNotFound, ErrorResponse{
				Error: "not_found", Message: "tenant not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "internal_error", Message: "failed to look up tenant",
		})
	}

	window := resolveWindow(c.QueryParam("window"))
	tenants, _, err := fetchAdminMetrics(ctx, h.metricsBackend, window, tenantID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "internal_error",
			Message: "failed to query metrics backend",
		})
	}

	// Pull the per-tenant row out of the map. If the metrics backend
	// returned no series for this tenant (every gauge/rate evaluated
	// to nothing in the requested window), surface a zero-valued row
	// rather than 404 — the tenant exists, just had no traffic.
	row, ok := tenants[tenantID]
	if !ok {
		row = TenantMetrics{TenantID: tenantID, Window: window}
	} else {
		// Defensive: ensure tenant_id and window are set even if the
		// stitch path lost them.
		row.TenantID = tenantID
		row.Window = window
	}
	return c.JSON(http.StatusOK, row)
}

// toAdminMetricsResponse serialises the stitched results for the
// cross-tenant endpoint. Drops the empty-tenant_id row from the
// per-tenant breakdown — that bucket exists so today's
// no-tenant-label series flow into the totals (see fetchAdminMetrics
// commentary), but it's not a real tenant.
func toAdminMetricsResponse(window string, tenants map[string]TenantMetrics, totals MetricsTotals) AdminMetricsResponse {
	rows := make([]TenantMetrics, 0, len(tenants))
	for tid, t := range tenants {
		if tid == "" {
			continue
		}
		// Defensive: ensure window is set on every row.
		if t.Window == "" {
			t.Window = window
		}
		rows = append(rows, t)
	}
	return AdminMetricsResponse{
		Window:  window,
		Tenants: rows,
		Totals:  totals,
	}
}

// bindOptionalReason extracts AdminActionRequest.reason from the
// request body. The body itself is optional per the spec; an empty
// body, malformed JSON, or a body without `reason` all collapse to
// "" — we never 400 here, the spec's `required: false` is permissive.
func bindOptionalReason(c echo.Context) string {
	var body AdminActionRequest
	// Bind error means the body was malformed JSON or had a wrong
	// content-type. Per spec the body is optional; treat as empty.
	_ = c.Bind(&body)
	return body.Reason
}

// parseIntDefault parses a non-empty string as int; on empty or
// parse failure returns def. Used for limit/offset query params.
func parseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

// mapDomainError turns a DomainError from a command handler into the
// matching HTTP status. Codes mirror those produced by
// internal/pkg/errors helpers: NOT_FOUND, INVALID_STATE,
// INVALID_INPUT, INTERNAL_ERROR.
func mapDomainError(c echo.Context, err error) error {
	var de *pkgerrors.DomainError
	if errors.As(err, &de) {
		switch de.Code {
		case "NOT_FOUND":
			return c.JSON(http.StatusNotFound, ErrorResponse{
				Error:   "not_found",
				Message: de.Message,
			})
		case "INVALID_STATE":
			return c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "invalid_state",
				Message: de.Message,
				Details: de.Details,
			})
		case "INVALID_INPUT":
			return c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "invalid_input",
				Message: de.Message,
				Details: de.Details,
			})
		}
	}
	// Fallback — unknown error type or DomainError code we don't map
	// explicitly. 500 is correct: command handlers wrap I/O failures
	// in errors.Internal which reaches here.
	return c.JSON(http.StatusInternalServerError, ErrorResponse{
		Error:   "internal_error",
		Message: "operation failed",
	})
}
