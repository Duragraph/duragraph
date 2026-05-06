// Package platform implements the self-service platform HTTP handlers
// under /api/platform/*. The contract lives in
// duragraph-spec/api/platform.yaml (Platform section) and
// duragraph-spec/auth/oauth.yml § /api/platform/me.
//
// Auth chain: each /api/platform/* request is gated by TenantMiddleware
// (NOT RequireTenant — pending users have valid JWTs with tenant_id=""
// and need to reach /me to render the awaiting-approval page). By the
// time a Handler method runs, the request context carries a verified
// user_id; tenant_id may or may not be present depending on the user's
// approval status.
//
// Spec field-name note: the canonical "me" response shape uses
// `user_id`, matching auth/oauth.yml § /api/platform/me.response_body_shape.
// api/platform.yaml's User schema names the same field `id` — that's a
// known divergence between the two spec files. Wave 1 wiring honours
// the explicit response_body_shape in oauth.yml; reconciliation is a
// future spec PR. Don't "fix" the JSON tag here without coordinating.
package platform

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/duragraph/duragraph/internal/domain/tenant"
	"github.com/duragraph/duragraph/internal/domain/user"
	"github.com/duragraph/duragraph/internal/infrastructure/http/middleware"
	pkgerrors "github.com/duragraph/duragraph/internal/pkg/errors"
)

// Handler is the HTTP layer for /api/platform/*. All dependencies are
// required; passing nil panics at construction time (matching the
// admin.NewHandler convention — fail fast at startup rather than lazy
// nil-deref on first request).
type Handler struct {
	userRepo   user.Repository
	tenantRepo tenant.Repository
}

// NewHandler constructs a Handler. The user and tenant repos must both
// be non-nil; passing nil panics. We follow the admin handler's
// fail-fast convention here rather than returning an error because
// these dependencies are wired exactly once at startup from main.go,
// where a panic surfaces as a startup-time crash that's easier to
// diagnose than a deferred 500 on first /me hit.
func NewHandler(userRepo user.Repository, tenantRepo tenant.Repository) *Handler {
	if userRepo == nil {
		panic("platform.NewHandler: userRepo must not be nil")
	}
	if tenantRepo == nil {
		panic("platform.NewHandler: tenantRepo must not be nil")
	}
	return &Handler{
		userRepo:   userRepo,
		tenantRepo: tenantRepo,
	}
}

// Register mounts the platform routes under the given Echo group. The
// caller is responsible for applying TenantMiddleware to the group
// BEFORE calling Register; the handler does not re-apply it.
func (h *Handler) Register(g *echo.Group) {
	g.GET("/me", h.Me)
}

// MeResponse mirrors the response_body_shape declared in
// duragraph-spec/auth/oauth.yml § /api/platform/me. TenantID is a
// pointer because pending users have no tenant — the spec models that
// as nullable, which JSON-omits to `null` (not a missing field).
type MeResponse struct {
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	Status    string    `json:"status"`
	TenantID  *string   `json:"tenant_id"`
	CreatedAt time.Time `json:"created_at"`
}

// errorResponse is the canonical error envelope used elsewhere in the
// platform-namespace handlers. Local copy rather than importing
// admin/dto's ErrorResponse to keep the platform package free of
// admin-specific imports — the JSON shape on the wire is identical.
type errorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// Me handles GET /api/platform/me.
//
// Returns the currently authenticated user's identity + status +
// (optional) tenant_id. Responds 401 in two failure modes:
//
//   - No user_id in the request context. TenantMiddleware is supposed
//     to populate this on every authenticated request; absence here
//     means either the middleware wasn't applied (route mis-wired) or
//     the request had no token. Defense-in-depth: 401 on either path.
//
//   - User row not found. The token references a user the platform
//     no longer has a record of (e.g. an admin deleted the row mid-
//     session). Treat as logged-out: 401, prompting the dashboard to
//     clear its cookie and route to /login.
//
// The tenant lookup is best-effort: a NotFound error for an approved
// user yields tenant_id=null in the response (data-invariant violation,
// but logging it is more useful than 500ing — the dashboard's pending/
// suspended pages render fine without a tenant_id, and a 500 would
// block the user from logging out).
func (h *Handler) Me(c echo.Context) error {
	userID, ok := middleware.UserIDFromCtx(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, errorResponse{
			Error:   "unauthorized",
			Message: "missing authenticated user",
		})
	}

	ctx := c.Request().Context()

	u, err := h.userRepo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, pkgerrors.ErrNotFound) {
			// Token references a nonexistent user — treat as
			// logged-out per the spec's "401 when neither
			// [cookie nor bearer] is present or valid".
			return c.JSON(http.StatusUnauthorized, errorResponse{
				Error:   "unauthorized",
				Message: "user not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Error:   "internal_error",
			Message: "failed to look up user",
		})
	}

	resp := MeResponse{
		UserID:    u.ID(),
		Email:     u.Email(),
		Role:      string(u.Role()),
		Status:    string(u.Status()),
		TenantID:  h.lookupTenantID(ctx, u),
		CreatedAt: u.CreatedAt(),
	}
	return c.JSON(http.StatusOK, resp)
}

// lookupTenantID returns a non-nil tenant_id only for approved users
// with an existing tenant row. Pending and suspended users — and any
// approved user whose tenant lookup fails — get nil (rendered as JSON
// null). Errors are logged but not surfaced to the client; /me must
// stay reachable for pending users and shouldn't 500 on a transient
// platform-DB blip during tenant lookup.
func (h *Handler) lookupTenantID(ctx context.Context, u *user.User) *string {
	if u.Status() != user.StatusApproved {
		return nil
	}
	t, err := h.tenantRepo.GetByUserID(ctx, u.ID())
	if err != nil {
		// Don't fail the endpoint. Approved-without-tenant is a
		// data-invariant violation (admin approval is meant to
		// provision atomically) but the dashboard renders fine with
		// a null tenant_id — the user can still log out. Log so the
		// case is diagnosable post-hoc.
		log.Printf("platform/me: tenant lookup failed for approved user %s: %v", u.ID(), err)
		return nil
	}
	id := t.ID()
	return &id
}
