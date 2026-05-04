package auth

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/labstack/echo/v4"

	authpkg "github.com/duragraph/duragraph/internal/infrastructure/auth"
	"github.com/duragraph/duragraph/internal/infrastructure/persistence/postgres"

	"github.com/duragraph/duragraph/internal/domain/tenant"
	"github.com/duragraph/duragraph/internal/domain/user"
	pkgerrors "github.com/duragraph/duragraph/internal/pkg/errors"
)

// SessionCookieName is the cookie that carries the platform JWT for
// browser sessions. Mirrors the constant in middleware/tenant.go (kept
// independent here to avoid a handler→middleware dep cycle; both
// MUST stay in sync with duragraph-spec/auth/oauth.yml § session.name).
const SessionCookieName = "duragraph_session"

// pgErrCodeUniqueViolation is SQLSTATE 23505 — what Postgres returns when
// an INSERT collides with a unique / primary-key constraint. Used by the
// callback decision tree to detect:
//   - bootstrap_lock contention (concurrent first-user signups)
//   - new_user race (concurrent callbacks for the same OAuth identity)
const pgErrCodeUniqueViolation = "23505"

// Routes for the post-callback redirect. Bare paths so the dashboard's
// router can claim them; the spec catalogues them in
// duragraph-spec/frontend/frontend.yml § platform.routes (out of scope
// for this PR — the engine just emits the redirect).
const (
	dashboardRoot         = "/"
	awaitingApprovalRoute = "/awaiting-approval"
	suspendedRoute        = "/suspended"
)

// supportedProviders is the closed set of providers we configure goth
// with. Matches duragraph-spec/auth/oauth.yml § providers.
var supportedProviders = map[string]struct{}{
	"google": {},
	"github": {},
}

// Config bundles the runtime configuration of the OAuth handler. All
// fields are required at construction time — there is no graceful
// fallback for any of them.
type Config struct {
	// SessionTTL is the JWT lifetime stamped on tokens minted at callback.
	// Spec default: 24h (auth/jwt.yml § exp.default_lifetime_seconds).
	SessionTTL time.Duration

	// CookieDomain is the value passed into Set-Cookie's Domain attribute.
	// Empty string means host-only (the default in dev). Sourced from
	// PLATFORM_COOKIE_DOMAIN env var per spec auth/oauth.yml § host.
	// MUST NOT include a scheme or port — Set-Cookie's Domain attribute
	// silently rejects those.
	CookieDomain string

	// CookieSecure controls the Secure attribute on the session cookie.
	// True in production (HTTPS-only), false in dev for plain-HTTP testing.
	CookieSecure bool

	// BaseURL is the platform's externally-facing URL (e.g.
	// "https://platform.duragraph.ai"). Used as the Origin/Referer reference
	// for logout CSRF checks. Must include scheme.
	BaseURL string

	// JWTSecret is the shared HMAC key used to sign tokens at callback and
	// to verify them at /api/auth/refresh. Must match the engine
	// middleware's secret (the spec calls this contract out in
	// auth/jwt.yml § signing.secret).
	JWTSecret []byte
}

// Handler implements the four OAuth-related HTTP endpoints. It is
// deliberately NOT wired into cmd/server/main.go in this PR — a follow-up
// PR will mount these routes alongside the future TenantMiddleware and
// /api/platform/me handler.
//
// The handler is safe for concurrent use: all dependencies are themselves
// concurrency-safe (pgxpool, repository implementations, exchanger).
type Handler struct {
	userRepo   user.Repository
	tenantRepo tenant.Repository
	migrator   *postgres.Migrator
	verifier   *authpkg.Verifier
	exchanger  Exchanger
	locker     BootstrapLocker

	cfg Config
}

// NewHandler constructs a Handler. All arguments are required — passing
// nil for any of the dependencies yields an error immediately rather than
// lazily on first request.
//
// The handler does NOT register goth providers itself — that's the caller's
// responsibility (it's package-level global state in goth that we'd rather
// not own from inside the handler constructor; tests in particular benefit
// from being able to register stubs once at TestMain time).
func NewHandler(
	userRepo user.Repository,
	tenantRepo tenant.Repository,
	migrator *postgres.Migrator,
	verifier *authpkg.Verifier,
	exchanger Exchanger,
	locker BootstrapLocker,
	cfg Config,
) (*Handler, error) {
	if userRepo == nil {
		return nil, fmt.Errorf("oauth handler: userRepo is required")
	}
	if tenantRepo == nil {
		return nil, fmt.Errorf("oauth handler: tenantRepo is required")
	}
	if migrator == nil {
		return nil, fmt.Errorf("oauth handler: migrator is required")
	}
	if verifier == nil {
		return nil, fmt.Errorf("oauth handler: verifier is required")
	}
	if exchanger == nil {
		return nil, fmt.Errorf("oauth handler: exchanger is required")
	}
	if locker == nil {
		return nil, fmt.Errorf("oauth handler: locker is required")
	}
	if cfg.SessionTTL <= 0 {
		return nil, fmt.Errorf("oauth handler: SessionTTL must be positive")
	}
	if len(cfg.JWTSecret) == 0 {
		return nil, fmt.Errorf("oauth handler: JWTSecret is required")
	}

	return &Handler{
		userRepo:   userRepo,
		tenantRepo: tenantRepo,
		migrator:   migrator,
		verifier:   verifier,
		exchanger:  exchanger,
		locker:     locker,
		cfg:        cfg,
	}, nil
}

// Login handles GET /api/auth/:provider/login. Initiates the OAuth dance
// by handing the request to gothic.BeginAuthHandler (via Exchanger), which
// 302-redirects to the provider's authorization endpoint and sets the
// `_gothic_session` state cookie.
//
// Returns 400 for an unknown provider; the redirect is performed by the
// exchanger and the response writer it owns at that point — the function
// simply returns nil after delegating.
func (h *Handler) Login(c echo.Context) error {
	provider := c.Param("provider")
	if _, ok := supportedProviders[provider]; !ok {
		return unknownProvider(c, provider)
	}

	// Delegate to the exchanger. gothic.BeginAuthHandler writes the
	// 302 directly to c.Response().Writer; we don't return JSON.
	if err := h.exchanger.BeginAuth(c.Response().Writer, c.Request(), provider); err != nil {
		return c.JSON(http.StatusBadGateway, errorBody("provider_error", "failed to start OAuth flow"))
	}
	return nil
}

// Callback handles GET /api/auth/:provider/callback. Exchanges the code
// for provider userinfo, runs the callback decision tree from
// duragraph-spec/auth/oauth.yml § callback_flow, mints a JWT (or doesn't,
// for suspended users), and 302-redirects the browser.
//
// Decision tree summary (full prose in oauth.yml):
//
//   - bootstrap_first_user → first user ever → admin + tenant + JWT → /
//   - existing_user_approved → JWT with role + tenant_id → /
//   - existing_user_pending  → pending JWT (tenant_id="") → /awaiting-approval
//   - existing_user_suspended → no JWT, no cookie → /suspended
//   - new_user → register pending → pending JWT → /awaiting-approval
func (h *Handler) Callback(c echo.Context) error {
	ctx := c.Request().Context()
	provider := c.Param("provider")
	if _, ok := supportedProviders[provider]; !ok {
		return unknownProvider(c, provider)
	}

	gu, err := h.exchanger.CompleteAuth(c.Response().Writer, c.Request(), provider)
	if err != nil {
		// gothic returns errors for state mismatch, missing code, network
		// failures during exchange, and provider userinfo errors. We
		// can't reliably distinguish them from a typed sentinel — gothic
		// wraps everything as plain errors.New strings. Keep the
		// classification crude (state-vs-other) by string-matching the
		// canonical "state token mismatch" message goth emits, and
		// otherwise treat as 502.
		if isStateError(err) {
			return c.JSON(http.StatusBadRequest, errorBody("state_mismatch", "OAuth state mismatch — possible CSRF or expired login attempt."))
		}
		return c.JSON(http.StatusBadGateway, errorBody("provider_exchange_failed", "OAuth provider returned an error. Try again."))
	}

	// Required identity fields. GitHub may return an empty Email from
	// gothic.CompleteUserAuth even though the github provider is supposed
	// to fall back to /user/emails — defensive check + 400.
	email := strings.TrimSpace(gu.Email)
	oauthID := strings.TrimSpace(gu.UserID)
	if email == "" {
		return c.JSON(http.StatusBadRequest, errorBody("no_verified_email", "OAuth provider returned no verified email. We require a verified email to create an account."))
	}
	if oauthID == "" {
		// Defensive — every supported provider returns a stable UserID.
		return c.JSON(http.StatusBadGateway, errorBody("provider_exchange_failed", "OAuth provider returned an empty user ID."))
	}

	// Look up the user by (provider, oauth_id). NotFound is the signal
	// to consider bootstrap or new_user; any other error is fatal.
	existing, err := h.userRepo.GetByOAuth(ctx, provider, oauthID)
	if err != nil && !pkgerrors.Is(err, pkgerrors.ErrNotFound) {
		return c.JSON(http.StatusInternalServerError, errorBody("internal_error", "failed to look up user"))
	}

	if existing != nil {
		return h.handleExistingUser(c, existing)
	}

	// No existing user. Decide bootstrap vs new_user.
	count, err := h.userRepo.CountAll(ctx)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody("internal_error", "failed to count users"))
	}

	if count == 0 {
		// Try to claim the bootstrap lock. If we win, run the bootstrap
		// branch; if we lose (unique-violation), fall through to new_user
		// — the winner has already created the admin/tenant pair.
		claimed, err := h.locker.TryClaim(ctx)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, errorBody("internal_error", "failed to claim bootstrap lock"))
		}
		if claimed {
			return h.handleBootstrap(c, provider, oauthID, email)
		}
		// Lost the race; fall through to new_user. Note that count was
		// observed as 0 BEFORE the lock claim — by now the winner has
		// inserted their admin row, so handleNewUser will see count >= 1
		// in subsequent paths and behave as a normal new_user signup.
	}

	return h.handleNewUser(c, provider, oauthID, email)
}

// Logout handles POST /api/auth/logout. Clears the session cookie. CSRF
// rules:
//
//   - Bearer-authenticated request (Authorization header present): no
//     ambient credential, no CSRF risk → skip the origin check, return 204.
//   - Cookie-authenticated request: require the Origin (or Referer)
//     header to match BaseURL's origin. Mismatch → 403.
//
// We don't do server-side session revocation (no blacklist in v0); the
// cookie clear is the entire effect. Per spec the JWT remains technically
// valid until its exp; that's the documented v0 lifecycle.
func (h *Handler) Logout(c echo.Context) error {
	authHeader := c.Request().Header.Get("Authorization")
	hasBearer := strings.HasPrefix(strings.ToLower(authHeader), "bearer ")

	if !hasBearer {
		// Cookie path → enforce same-origin CSRF defence.
		if !h.originMatchesBaseURL(c.Request()) {
			return c.JSON(http.StatusForbidden, errorBody("csrf_check_failed", "CSRF check failed"))
		}
	}

	h.clearSessionCookie(c)
	return c.NoContent(http.StatusNoContent)
}

// Refresh handles POST /api/auth/refresh. Bearer-only by contract — cookie
// rotation is middleware's job, not this endpoint's. Reads + verifies the
// current token, mints a new one with the same identity claims and a fresh
// `iat`/`exp`, and returns it in JSON.
func (h *Handler) Refresh(c echo.Context) error {
	authHeader := c.Request().Header.Get("Authorization")
	const prefix = "bearer "
	if len(authHeader) <= len(prefix) || !strings.EqualFold(authHeader[:len(prefix)], prefix) {
		return c.JSON(http.StatusUnauthorized, errorBody("unauthorized", "missing bearer token"))
	}
	tokenString := strings.TrimSpace(authHeader[len(prefix):])
	if tokenString == "" {
		return c.JSON(http.StatusUnauthorized, errorBody("unauthorized", "missing bearer token"))
	}

	claims, err := h.verifier.Verify(tokenString)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, errorBody("unauthorized", "invalid bearer token"))
	}

	newToken, err := authpkg.IssueJWT(
		h.cfg.JWTSecret,
		claims.UserID,
		claims.Email,
		claims.Role,
		claims.TenantID,
		h.cfg.SessionTTL,
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody("internal_error", "failed to issue token"))
	}

	exp := time.Now().Add(h.cfg.SessionTTL).Unix()
	return c.JSON(http.StatusOK, map[string]any{
		"token": newToken,
		"exp":   exp,
	})
}

// ---- callback decision-tree branches -------------------------------------

// handleExistingUser dispatches on the user's status.
func (h *Handler) handleExistingUser(c echo.Context, u *user.User) error {
	switch u.Status() {
	case user.StatusApproved:
		return h.handleApproved(c, u)
	case user.StatusPending:
		return h.handlePending(c, u)
	case user.StatusSuspended:
		return h.handleSuspended(c)
	default:
		// Defense-in-depth — table-level CHECK should make this unreachable.
		return c.JSON(http.StatusInternalServerError, errorBody("internal_error", "unknown user status"))
	}
}

// handleApproved mints a full token (role + tenant_id) and redirects to
// the dashboard root.
func (h *Handler) handleApproved(c echo.Context, u *user.User) error {
	ctx := c.Request().Context()
	tnt, err := h.tenantRepo.GetByUserID(ctx, u.ID())
	if err != nil {
		// Approved-but-no-tenant is a data invariant violation (the
		// approval path is supposed to provision a tenant atomically with
		// the status flip). Surface as 500 rather than silently degrading
		// to a pending JWT.
		return c.JSON(http.StatusInternalServerError, errorBody("internal_error", "approved user has no tenant"))
	}

	token, err := authpkg.IssueJWT(
		h.cfg.JWTSecret,
		u.ID(),
		u.Email(),
		string(u.Role()),
		tnt.ID(),
		h.cfg.SessionTTL,
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody("internal_error", "failed to issue token"))
	}

	h.setSessionCookie(c, token)
	return c.Redirect(http.StatusFound, dashboardRoot)
}

// handlePending mints a token with empty tenant_id and redirects to
// /awaiting-approval.
func (h *Handler) handlePending(c echo.Context, u *user.User) error {
	token, err := authpkg.IssueJWT(
		h.cfg.JWTSecret,
		u.ID(),
		u.Email(),
		string(user.RoleUser),
		"",
		h.cfg.SessionTTL,
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody("internal_error", "failed to issue token"))
	}

	h.setSessionCookie(c, token)
	return c.Redirect(http.StatusFound, awaitingApprovalRoute)
}

// handleSuspended redirects to /suspended without minting a token or
// touching the cookie. Spec is explicit: "Do NOT mint a JWT, do NOT set
// a session cookie."
func (h *Handler) handleSuspended(c echo.Context) error {
	return c.Redirect(http.StatusFound, suspendedRoute)
}

// handleBootstrap runs the auto-admin + auto-tenant path for the very-
// first-user. Caller MUST have claimed the bootstrap_lock row before
// calling this. Order of side effects (per spec § callback_flow.bootstrap_first_user):
//
//  1. RegisterUser(isFirstUser=true) — emits UserSignedUp +
//     UserPromotedToAdmin + UserApproved on the aggregate.
//  2. userRepo.Save → INSERT platform.users with role=admin, status=approved.
//  3. NewTenant(userID) — emits TenantPending.
//  4. migrator.ProvisionTenant — CREATE DATABASE + apply tenant migrations.
//  5. migrator.MigrateTenant (idempotent re-call) to capture schemaVersion.
//  6. tenant.StartProvisioning + tenant.Approve(approvedByUserID=user.ID(),
//     schemaVersion).
//  7. tenantRepo.Save.
//  8. Mint JWT, set cookie, redirect to /.
//
// The user.ID() is intentionally used as approvedByUserID — that's the
// documented bootstrap exception to the self-action guard (the User
// aggregate's domain methods enforce it for normal flow but RegisterUser
// already records UserApproved directly with that ID; here we mirror the
// same convention for tenant.Approve).
func (h *Handler) handleBootstrap(c echo.Context, provider, oauthID, email string) error {
	ctx := c.Request().Context()

	u, err := user.RegisterUser(email, provider, oauthID, true)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody("internal_error", fmt.Sprintf("failed to register user: %v", err)))
	}
	if err := h.userRepo.Save(ctx, u); err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody("internal_error", "failed to save user"))
	}

	t, err := tenant.NewTenant(u.ID())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody("internal_error", "failed to create tenant aggregate"))
	}

	if err := h.migrator.ProvisionTenant(ctx, t.ID()); err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody("internal_error", "failed to provision tenant db"))
	}
	version, err := h.migrator.MigrateTenant(ctx, t.ID())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody("internal_error", "failed to read tenant schema version"))
	}

	if err := t.StartProvisioning(); err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody("internal_error", "failed to transition tenant to provisioning"))
	}
	if err := t.Approve(u.ID(), int(version)); err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody("internal_error", "failed to approve tenant"))
	}
	if err := h.tenantRepo.Save(ctx, t); err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody("internal_error", "failed to save tenant"))
	}

	token, err := authpkg.IssueJWT(
		h.cfg.JWTSecret,
		u.ID(),
		u.Email(),
		string(user.RoleAdmin),
		t.ID(),
		h.cfg.SessionTTL,
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody("internal_error", "failed to issue token"))
	}

	h.setSessionCookie(c, token)
	return c.Redirect(http.StatusFound, dashboardRoot)
}

// handleNewUser registers a brand-new pending user. Race-handling: the
// (oauth_provider, oauth_id) unique constraint on platform.users
// guarantees at most one row per OAuth identity. Two concurrent callbacks
// for the same identity will collide on INSERT; the loser detects
// SQLSTATE 23505, re-fetches the row via GetByOAuth, and proceeds with
// that aggregate. This implements the spec's "atomic upsert" semantics
// without a refactor of UserRepository.Save (which today only does
// INSERT, no ON CONFLICT).
func (h *Handler) handleNewUser(c echo.Context, provider, oauthID, email string) error {
	ctx := c.Request().Context()

	u, err := user.RegisterUser(email, provider, oauthID, false)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody("internal_error", fmt.Sprintf("failed to register user: %v", err)))
	}

	if err := h.userRepo.Save(ctx, u); err != nil {
		// Race: another callback inserted the same (provider, oauth_id)
		// between our GetByOAuth probe and this INSERT. Look up the
		// winner's row and treat it as the existing user.
		if isUniqueViolation(err) {
			existing, getErr := h.userRepo.GetByOAuth(ctx, provider, oauthID)
			if getErr != nil {
				return c.JSON(http.StatusInternalServerError, errorBody("internal_error", "failed to recover from race"))
			}
			return h.handleExistingUser(c, existing)
		}
		return c.JSON(http.StatusInternalServerError, errorBody("internal_error", "failed to save user"))
	}

	// Fresh pending user; emit a pending JWT and redirect.
	return h.handlePending(c, u)
}

// ---- helpers --------------------------------------------------------------

// setSessionCookie writes the duragraph_session cookie on the response.
// Attribute set per spec auth/oauth.yml § session.primary_transport:
// HttpOnly, Secure, SameSite=Lax, Path=/, Max-Age=86400.
func (h *Handler) setSessionCookie(c echo.Context, token string) {
	cookie := &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		Domain:   h.cfg.CookieDomain, // empty → host-only (correct for dev)
		MaxAge:   int(h.cfg.SessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   h.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	}
	c.SetCookie(cookie)
}

// clearSessionCookie writes a Max-Age=0 cookie with the same attributes
// used at issue time, instructing the browser to delete it. Path/Domain
// MUST match the issued cookie or the browser ignores the deletion.
func (h *Handler) clearSessionCookie(c echo.Context) {
	cookie := &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		Domain:   h.cfg.CookieDomain,
		MaxAge:   -1, // -1 produces "Max-Age=0" via net/http (RFC 6265).
		HttpOnly: true,
		Secure:   h.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	}
	c.SetCookie(cookie)
}

// originMatchesBaseURL implements the cookie-logout CSRF defence: the
// request's Origin header (or, if absent, Referer) MUST match the origin
// of cfg.BaseURL.
//
// If cfg.BaseURL is unset (e.g. in tests that don't configure it), the
// check passes — there's no reference origin to compare against, and the
// caller has explicitly opted out of the safeguard. Production deployments
// MUST set PLATFORM_BASE_URL.
func (h *Handler) originMatchesBaseURL(r *http.Request) bool {
	if h.cfg.BaseURL == "" {
		return true
	}
	want, err := url.Parse(h.cfg.BaseURL)
	if err != nil || want.Host == "" {
		return false
	}

	if origin := r.Header.Get("Origin"); origin != "" {
		got, err := url.Parse(origin)
		if err != nil {
			return false
		}
		return got.Scheme == want.Scheme && got.Host == want.Host
	}

	if referer := r.Header.Get("Referer"); referer != "" {
		got, err := url.Parse(referer)
		if err != nil {
			return false
		}
		return got.Scheme == want.Scheme && got.Host == want.Host
	}

	// Cookie auth without Origin or Referer — block. Modern browsers send
	// at least one for state-changing POSTs; absence is suspicious.
	return false
}

// errorBody builds an error response payload that matches the dto.ErrorResponse
// shape used elsewhere in the engine (Error, Message). Local helper rather
// than importing dto to avoid pulling the LangGraph DTO surface into this
// package; the JSON shape on the wire is identical.
func errorBody(code, message string) map[string]string {
	return map[string]string{
		"error":   code,
		"message": message,
	}
}

// unknownProvider returns the canonical 400 for an unsupported provider,
// per spec auth/oauth.yml § errors.unknown_provider.
func unknownProvider(c echo.Context, provider string) error {
	return c.JSON(http.StatusBadRequest, errorBody(
		"unknown_provider",
		fmt.Sprintf("Unknown OAuth provider: %s. Supported: google, github.", provider),
	))
}

// isUniqueViolation reports whether err (or anything in its chain) is a
// pgconn.PgError with SQLSTATE 23505 (unique_violation). Used by both the
// bootstrap-lock claim and the new_user race handler.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == pgErrCodeUniqueViolation
	}
	return false
}

// isStateError matches gothic's state-token-mismatch error by string. The
// goth library doesn't expose a typed sentinel for this; the error text
// is stable across recent versions ("state token mismatch"). String-match
// is a deliberate trade-off — the only alternative is treating ALL
// CompleteUserAuth errors as 502, which loses the 400-vs-502 split the
// spec calls out under § errors.state_mismatch / § errors.provider_exchange_failed.
func isStateError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "state") && strings.Contains(msg, "mismatch")
}
