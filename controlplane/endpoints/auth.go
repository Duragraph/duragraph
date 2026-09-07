// Hand-written auth endpoints (the OAuth surface at /api/auth/*). Routes
// generated into auth_gen.go; bodies live here. Targets the PLATFORM database
// (endpoints.yaml: group auth, db: platform).
//
// The provider exchange itself is NOT reimplemented: the Exchanger seam and its
// goth-backed implementation already exist in
// internal/infrastructure/http/handlers/auth and are imported here. That
// package bridges Echo's :provider path param to gothic's context-based
// provider lookup — a detail that is easy to get wrong (gothic's own lookup is
// gorilla/mux-flavoured and silently falls back to a ?provider= query param
// that this surface never sets) and that there is no reason to get wrong twice.
//
// What IS different here from that older implementation is persistence: it
// wrote through DDD repositories, whereas the control plane is event-sourced.
// Every branch that creates or changes a user goes through writeTx so the rows,
// the events and the outbox notify commit as one unit.
package endpoints

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/labstack/echo/v4"
	"github.com/markbates/goth"

	"github.com/duragraph/duragraph/controlplane/eventstore"
	authpkg "github.com/duragraph/duragraph/internal/infrastructure/auth"
	authhandlers "github.com/duragraph/duragraph/internal/infrastructure/http/handlers/auth"
)

// supportedProviders is a CLOSED set. goth will happily attempt any provider
// name it has been configured with; pinning the set here means adding a
// provider is a deliberate change to this file rather than a side effect of
// configuration.
var supportedProviders = map[string]struct{}{
	"google": {},
	"github": {},
}

// oauthExchanger is the provider-exchange seam. It is satisfied by
// authhandlers.Exchanger (and therefore by authhandlers.GothExchanger); tests
// substitute a stub because a real OAuth round trip cannot run in-process.
type oauthExchanger interface {
	BeginAuth(w http.ResponseWriter, r *http.Request, provider string) error
	CompleteAuth(w http.ResponseWriter, r *http.Request, provider string) (goth.User, error)
}

// exchanger returns the configured exchange implementation, defaulting to the
// real goth-backed one. Defaulting lazily (rather than requiring the
// composition root to set it) keeps the zero-value Server usable.
func (s *Server) exchanger() oauthExchanger {
	if s.OAuth != nil {
		return s.OAuth
	}
	return authhandlers.NewGothExchanger()
}

// authUnavailable reports whether the surface can operate at all.
//
// With no JWTSecret there is no key to sign or verify with. Signing with an
// empty key would produce tokens that ANY other empty-key deployment could
// forge, so this fails closed with 503 rather than minting them.
func (s *Server) authUnavailable() error {
	if len(s.Auth.JWTSecret) == 0 {
		return echo.NewHTTPError(http.StatusServiceUnavailable,
			"authentication is not configured on this deployment")
	}
	return nil
}

// AuthLogin starts the OAuth flow.
// GET /api/auth/{provider}/login -> 302 to the provider / 400 / 502.
func (s *Server) AuthLogin(c echo.Context) error {
	if err := s.authUnavailable(); err != nil {
		return err
	}
	provider, err := s.checkProvider(c)
	if err != nil {
		return err
	}
	// BeginAuth writes the 302 and the state cookie directly onto the
	// ResponseWriter, so there is nothing further to return on success.
	if err := s.exchanger().BeginAuth(c.Response().Writer, c.Request(), provider); err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "provider_error")
	}
	return nil
}

// checkProvider validates :provider twice: against the closed set above, and
// against what goth actually has registered. Both misses are 400
// unknown_provider — a provider that is supported in principle but unregistered
// (its client id/secret was never configured) is, from the caller's side,
// simply not available, and reporting 502 would blame the provider for a local
// configuration gap.
func (s *Server) checkProvider(c echo.Context) (string, error) {
	provider := strings.ToLower(strings.TrimSpace(c.Param("provider")))
	if _, ok := supportedProviders[provider]; !ok {
		return "", echo.NewHTTPError(http.StatusBadRequest, "unknown_provider")
	}
	// A stub exchanger (tests) means goth is not configured at all; the
	// registration check only applies to the real one.
	if s.OAuth == nil {
		if _, err := goth.GetProvider(provider); err != nil {
			return "", echo.NewHTTPError(http.StatusBadRequest, "unknown_provider")
		}
	}
	return provider, nil
}

// AuthCallback completes the OAuth flow and establishes the session.
// GET /api/auth/{provider}/callback -> 302 / 400 / 502 / 500.
//
// The decision tree (endpoints.yaml auth.callback branches):
//
//	known identity  -> existing: mint a JWT, write nothing
//	unknown, no users at all -> bootstrap: first user becomes admin+approved
//	unknown, users exist     -> new_user: pending, awaiting operator approval
func (s *Server) AuthCallback(c echo.Context) error {
	ctx := c.Request().Context()
	if err := s.authUnavailable(); err != nil {
		return err
	}
	provider, err := s.checkProvider(c)
	if err != nil {
		return err
	}

	gu, err := s.exchanger().CompleteAuth(c.Response().Writer, c.Request(), provider)
	if err != nil {
		// goth exposes no typed sentinel for a state mismatch, so this is a
		// string match — the same compromise the existing implementation makes.
		// A state mismatch is the CSRF failure case and is the caller's
		// problem (400); anything else is the provider's (502).
		low := strings.ToLower(err.Error())
		if strings.Contains(low, "state") && strings.Contains(low, "mismatch") {
			return echo.NewHTTPError(http.StatusBadRequest, "state_mismatch")
		}
		return echo.NewHTTPError(http.StatusBadGateway, "provider_exchange_failed")
	}

	email := strings.TrimSpace(gu.Email)
	oauthID := strings.TrimSpace(gu.UserID)
	if email == "" {
		// GitHub can return an empty email when every address is private.
		// Without one there is no account to key on.
		return echo.NewHTTPError(http.StatusBadRequest, "no_verified_email")
	}
	if oauthID == "" {
		return echo.NewHTTPError(http.StatusBadGateway, "provider_exchange_failed")
	}

	existing, err := s.findUserByOAuth(ctx, provider, oauthID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if existing != nil {
		return s.completeExistingUser(c, existing)
	}
	return s.completeNewIdentity(c, provider, oauthID, email)
}

// platformUser is the subset of a users row the auth flow needs.
type platformUser struct {
	ID       uuid.UUID
	Email    string
	Role     string
	Status   string
	TenantID *uuid.UUID
}

func (s *Server) findUserByOAuth(ctx context.Context, provider, oauthID string) (*platformUser, error) {
	var u platformUser
	err := s.Platform.QueryRow(ctx, `
		SELECT u.id, u.email, u.role, u.status, t.id
		FROM users u
		LEFT JOIN tenants t ON t.user_id = u.id
		WHERE u.oauth_provider = $1 AND u.oauth_id = $2`,
		provider, oauthID).Scan(&u.ID, &u.Email, &u.Role, &u.Status, &u.TenantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// completeExistingUser handles a known identity: no writes at all, just a
// session (or deliberately none, for a suspended user).
func (s *Server) completeExistingUser(c echo.Context, u *platformUser) error {
	switch u.Status {
	case "approved":
		tenantID := ""
		if u.TenantID != nil {
			tenantID = u.TenantID.String()
		}
		// An approved user with no tenant is a broken invariant, not a
		// degraded login: their JWT would carry no tenant and every tenant
		// -scoped call would fail confusingly later. Fail loudly, here.
		if tenantID == "" {
			return echo.NewHTTPError(http.StatusInternalServerError,
				"approved user has no tenant")
		}
		if _, err := s.issueSession(c, u.ID.String(), u.Email, u.Role, tenantID); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.Redirect(http.StatusFound, "/")

	case "pending":
		// Role is pinned to "user" and the tenant left empty regardless of what
		// the row says: a pending account must not carry admin authority in its
		// token before an operator has approved it.
		if _, err := s.issueSession(c, u.ID.String(), u.Email, "user", ""); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.Redirect(http.StatusFound, "/awaiting-approval")

	case "suspended":
		// NO token and NO cookie — a suspended user must leave the callback
		// with strictly less authority than they arrived with.
		return c.Redirect(http.StatusFound, "/suspended")

	default:
		// Unreachable while the users CHECK holds; defence in depth.
		return echo.NewHTTPError(http.StatusInternalServerError, "unknown user status")
	}
}

// completeNewIdentity handles an identity with no users row yet: either the
// installation's first user (bootstrap) or a normal pending signup.
func (s *Server) completeNewIdentity(c echo.Context, provider, oauthID, email string) error {
	ctx := c.Request().Context()

	var count int
	if err := s.Platform.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&count); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	bootstrap := false
	if count == 0 {
		claimed, err := s.claimBootstrap(ctx)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		// Losing the claim means a concurrent first login already became admin.
		// Falling through to the pending path is correct — we are now simply
		// the second user.
		bootstrap = claimed
	}

	created, err := s.insertIdentity(ctx, provider, oauthID, email, bootstrap)
	if err != nil {
		if isUniqueViolation(err) {
			// A concurrent callback for the SAME identity inserted first. Re-read
			// and treat this as a normal existing-user login rather than failing
			// a legitimate sign-in on a race.
			existing, ferr := s.findUserByOAuth(ctx, provider, oauthID)
			if ferr != nil || existing == nil {
				return echo.NewHTTPError(http.StatusInternalServerError, "signup raced and could not be resolved")
			}
			return s.completeExistingUser(c, existing)
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return s.completeExistingUser(c, created)
}

// claimBootstrap atomically elects the first user as admin.
//
// count(*)==0 above is only an advisory probe — two simultaneous first logins
// can both read zero. The election itself is this insert: bootstrap_lock's
// primary key is a BOOLEAN pinned TRUE by CHECK, so the table admits exactly
// one row for the lifetime of the installation. The winner gets nil; every
// later attempt gets a unique violation, which is a normal outcome and not an
// error.
func (s *Server) claimBootstrap(ctx context.Context) (bool, error) {
	_, err := s.Platform.Exec(ctx, `INSERT INTO bootstrap_lock (id) VALUES (true)`)
	if err == nil {
		return true, nil
	}
	if isUniqueViolation(err) {
		return false, nil
	}
	return false, err
}

// insertIdentity creates the users + tenants rows and emits the branch's
// events, all in one transaction.
func (s *Server) insertIdentity(ctx context.Context, provider, oauthID, email string, bootstrap bool) (*platformUser, error) {
	role, userStatus := "user", "pending"
	events := []string{"user.signed_up", "tenant.pending"}
	if bootstrap {
		role, userStatus = "admin", "approved"
		events = []string{"user.signed_up", "user.promoted_to_admin", "user.approved", "tenant.pending", "tenant.provisioning"}
	}

	out := &platformUser{Email: email, Role: role, Status: userStatus}
	err := s.writeTxDeferred(ctx, s.Platform, func(tx pgx.Tx) ([]eventstore.Event, error) {
		if err := tx.QueryRow(ctx, `
			INSERT INTO users (oauth_provider, oauth_id, email, role, status)
			VALUES ($1,$2,$3,$4,$5) RETURNING id`,
			provider, oauthID, email, role, userStatus).Scan(&out.ID); err != nil {
			return nil, err
		}

		// db_name is NOT NULL UNIQUE and the spec's format is tenant_<32hex>.
		// It is assigned at signup even though the database itself is only
		// created later by the provisioner, because the name has to be stable
		// from the moment the tenant row exists.
		dbName := "tenant_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		var tid uuid.UUID
		if err := tx.QueryRow(ctx, `
			INSERT INTO tenants (user_id, db_name, status)
			VALUES ($1,$2,'pending') RETURNING id`, out.ID, dbName).Scan(&tid); err != nil {
			return nil, err
		}
		out.TenantID = &tid

		evs := make([]eventstore.Event, 0, len(events))
		for _, et := range events {
			if strings.HasPrefix(et, "user.") {
				evs = append(evs, eventstore.Event{
					AggregateType: "User",
					AggregateID:   out.ID,
					EventType:     et,
					Payload: mustJSON(map[string]any{
						"user_id": out.ID.String(),
						"email":   email,
						"role":    role,
						"status":  userStatus,
					}),
				})
				continue
			}
			evs = append(evs, eventstore.Event{
				AggregateType: "Tenant",
				AggregateID:   tid,
				EventType:     et,
				Payload: mustJSON(map[string]any{
					"tenant_id": tid.String(),
					"user_id":   out.ID.String(),
					"db_name":   dbName,
				}),
			})
		}
		return evs, nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// isUniqueViolation reports whether err is a Postgres unique-constraint
// violation (SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// AuthLogout clears the session cookie.
// POST /api/auth/logout -> 204 / 403.
//
// There is no server-side revocation: the JWT stays valid until it expires.
// That is the documented v0 behaviour and the reason the TTL is 24h rather
// than longer.
func (s *Server) AuthLogout(c echo.Context) error {
	// A Bearer credential is presented explicitly by a client that had to hold
	// the token, so there is no ambient authority to abuse and CSRF does not
	// apply. A cookie-based logout is exactly the ambient case, so it must
	// prove same-origin.
	if bearerToken(c) == "" {
		if err := s.checkSameOrigin(c); err != nil {
			return err
		}
	}
	s.clearSession(c)
	return c.NoContent(http.StatusNoContent)
}

// checkSameOrigin enforces the logout CSRF rule: Origin (or Referer) must match
// the configured BaseURL on scheme and host.
//
// FAIL-CLOSED. If neither header is present there is nothing to compare, and a
// cross-site form post is precisely the request that would arrive without them,
// so the absence is treated as a failure rather than waved through.
func (s *Server) checkSameOrigin(c echo.Context) error {
	if s.Auth.BaseURL == "" {
		return echo.NewHTTPError(http.StatusForbidden, "csrf_check_failed")
	}
	base, err := url.Parse(s.Auth.BaseURL)
	if err != nil {
		return echo.NewHTTPError(http.StatusForbidden, "csrf_check_failed")
	}
	raw := c.Request().Header.Get(echo.HeaderOrigin)
	if raw == "" {
		raw = c.Request().Header.Get("Referer")
	}
	if raw == "" {
		return echo.NewHTTPError(http.StatusForbidden, "csrf_check_failed")
	}
	got, err := url.Parse(raw)
	if err != nil || got.Scheme != base.Scheme || got.Host != base.Host {
		return echo.NewHTTPError(http.StatusForbidden, "csrf_check_failed")
	}
	return nil
}

// AuthRefreshResponse is the refresh body: the new token and its expiry, so a
// client can schedule the next refresh without decoding the JWT itself.
type AuthRefreshResponse struct {
	Token string `json:"token"`
	Exp   int64  `json:"exp"`
}

// AuthRefresh re-issues a JWT with the same claims and a later expiry.
// POST /api/auth/refresh -> 200 AuthRefreshResponse / 401 / 503.
//
// BEARER ONLY, deliberately: the cookie is not read. Refresh is an API-client
// operation, and honouring an ambient cookie here would let any cross-site page
// silently extend a browser session.
func (s *Server) AuthRefresh(c echo.Context) error {
	if err := s.authUnavailable(); err != nil {
		return err
	}
	raw := bearerToken(c)
	if raw == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
	}
	claims, err := authpkg.VerifyJWT(s.Auth.JWTSecret, raw)
	if err != nil {
		// The specific reason (expired vs forged vs malformed) is withheld —
		// it is not information a caller should be able to probe for.
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid session")
	}

	token, err := authpkg.IssueJWT(s.Auth.JWTSecret,
		claims.UserID, claims.Email, claims.Role, claims.TenantID, s.Auth.ttl())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	// Read the expiry back off the freshly-signed token rather than recomputing
	// now()+ttl: the two can differ by a second, and the value a client
	// schedules against must be the one actually inside the token.
	fresh, err := authpkg.VerifyJWT(s.Auth.JWTSecret, token)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, AuthRefreshResponse{Token: token, Exp: fresh.ExpiresAt.Unix()})
}
