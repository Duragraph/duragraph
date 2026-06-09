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

// RegisterAuth mounts the auth endpoints on g (the /api/v1 group).
func (s *Server) RegisterAuth(g *echo.Group) {
	g.GET("/api/auth/:provider/login", s.AuthLogin)
	g.GET("/api/auth/:provider/callback", s.AuthCallback)
	g.POST("/api/auth/logout", s.AuthLogout)
	g.POST("/api/auth/refresh", s.AuthRefresh)
}

// AuthLogin — GET /api/auth/{provider}/login  (kind: special)
//   - Set goth CSRF state cookie
//   - 302 redirect to provider authorization URL
func (s *Server) AuthLogin(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	// SPECIAL: bespoke (auth redirect / health / metrics). Fill in.
	//   Set goth CSRF state cookie
	//   302 redirect to provider authorization URL
	return echo.NewHTTPError(http.StatusNotImplemented, "handler not implemented")
}

// AuthCallback — GET /api/auth/{provider}/callback  (kind: write)
//   - Validate goth session state cookie
//   - Exchange code (provider token exchange)
//   - Fetch userinfo (email, oauth_id)
//   - UPSERT users ON CONFLICT (oauth_provider, oauth_id) DO UPDATE SET email
//   - branch: bootstrap | new_user | existing (see branches)
//   - Set cookie duragraph_session = JWT
//   - 302 redirect to dashboard / awaiting-approval / suspended
func (s *Server) AuthCallback(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	var req map[string]any // TODO: bind OpenAPI type (AuthCallback request schema)
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	aggID := uuid.New() // TODO: new id for create; parse from path param for update/cancel/etc.
	events := []Event{
		{AggregateType: "User", AggregateID: aggID, EventType: "user.signed_up"},
		{AggregateType: "User", AggregateID: aggID, EventType: "user.promoted_to_admin"},
		{AggregateType: "User", AggregateID: aggID, EventType: "user.approved"},
		{AggregateType: "Tenant", AggregateID: aggID, EventType: "tenant.pending"},
		{AggregateType: "Tenant", AggregateID: aggID, EventType: "tenant.provisioning"},
	}
	if err := s.writeTx(ctx, s.Platform, events, func(tx pgx.Tx) error {
		// TODO projection write:
		//   Validate goth session state cookie
		//   Exchange code (provider token exchange)
		//   Fetch userinfo (email, oauth_id)
		//   UPSERT users ON CONFLICT (oauth_provider, oauth_id) DO UPDATE SET email
		//   branch: bootstrap | new_user | existing (see branches)
		//   Set cookie duragraph_session = JWT
		//   302 redirect to dashboard / awaiting-approval / suspended
		return nil
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{}) // TODO: return OpenAPI response type
}

// AuthLogout — POST /api/auth/logout  (kind: special)
//   - Clear cookie: Set-Cookie duragraph_session=; Max-Age=0
func (s *Server) AuthLogout(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	// SPECIAL: bespoke (auth redirect / health / metrics). Fill in.
	//   Clear cookie: Set-Cookie duragraph_session=; Max-Age=0
	return echo.NewHTTPError(http.StatusNotImplemented, "handler not implemented")
}

// AuthRefresh — POST /api/auth/refresh  (kind: special)
//   - Validate current JWT (signature + expiry)
//   - Mint new JWT (same claims, new exp)
func (s *Server) AuthRefresh(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	// SPECIAL: bespoke (auth redirect / health / metrics). Fill in.
	//   Validate current JWT (signature + expiry)
	//   Mint new JWT (same claims, new exp)
	return echo.NewHTTPError(http.StatusNotImplemented, "handler not implemented")
}
