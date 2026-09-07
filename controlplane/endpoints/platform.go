// Hand-written platform endpoints (the self-service surface at
// /api/platform/*). Route generated into platform_gen.go; body lives here.
// Targets the PLATFORM database (endpoints.yaml: group platform, db: platform)
// — users/tenants, not the per-tenant workflow schema.
package endpoints

import (
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

// PlatformMeResponse is the /api/platform/me body. The shape mirrors the one
// already served by internal/infrastructure/http/handlers/platform (MeResponse)
// so a dashboard can talk to either surface unchanged: user_id (NOT id — see
// that package's header note on the platform.yaml ↔ oauth.yml divergence),
// email, role, status, tenant_id, created_at.
//
// TenantID is a POINTER, and deliberately not `omitempty`: "this user has no
// tenant yet" is a real state (a pending signup awaiting operator approval),
// and the dashboard distinguishes an explicit null from a missing field when
// it decides between the app shell and the awaiting-approval page.
type PlatformMeResponse struct {
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	Status    string    `json:"status"`
	TenantID  *string   `json:"tenant_id"`
	CreatedAt time.Time `json:"created_at"`
}

// PlatformMe returns the caller's identity, FRESH FROM THE DATABASE.
// GET /api/platform/me -> 200 PlatformMeResponse / 401.
//
// The freshness is the whole point of the endpoint. A session token is a
// bearer snapshot taken at login and valid for 24h (DefaultSessionTTL), so
// everything an operator does in between — approve, reject, suspend, promote
// to admin, provision the tenant — is invisible to the token until it expires.
// Answering /me from the claims alone would therefore tell a user suspended
// five minutes ago that they are still approved, and tell a user approved five
// minutes ago that they are still pending. So the claims are used for one
// thing only — WHICH user is asking — and every attribute that can change
// (status, role, email, tenant_id) is re-read from the users/tenants rows.
//
// Note that `status` is not even carried in the token: authpkg.Claims is
// {user_id, tenant_id, role, email} (internal/infrastructure/auth/jwt.go), so
// endpoints.yaml's "Read JWT claims (user_id, email, role, status, tenant_id)"
// is only satisfiable via the row read the very next step prescribes.
//
// 401 (not 404) when the row is gone: the token references a user the platform
// no longer has — a deleted account mid-session — which is a dead session, not
// a missing resource. Saying 401 is what makes the dashboard clear its cookie
// and route to /login instead of rendering an error page for a ghost.
func (s *Server) PlatformMe(c echo.Context) error {
	ctx := c.Request().Context()
	claims, err := s.requireSession(c)
	if err != nil {
		return err
	}
	// The subject is a users.id (uuid). A syntactically impossible one can only
	// come from a token this server would not have minted, so it is treated as
	// an invalid session rather than passed to Postgres to reject as a 500.
	uid, err := uuid.Parse(claims.UserID)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid session")
	}

	// One round trip: the user row plus its tenant (LEFT JOIN — tenants.user_id
	// is UNIQUE, so this can match at most one row, and no match simply leaves
	// tenant_id null for a user whose tenant has not been created yet).
	var resp PlatformMeResponse
	err = s.Platform.QueryRow(ctx, `
		SELECT u.id::text, u.email, u.role, u.status, t.id::text, u.created_at
		FROM users u
		LEFT JOIN tenants t ON t.user_id = u.id
		WHERE u.id = $1`, uid).
		Scan(&resp.UserID, &resp.Email, &resp.Role, &resp.Status, &resp.TenantID, &resp.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}
