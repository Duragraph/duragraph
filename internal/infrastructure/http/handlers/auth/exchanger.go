// Package auth implements the platform OAuth login/callback/logout/refresh
// HTTP endpoints. The endpoints are the surface described in
// duragraph-spec/auth/oauth.yml and the JWT they mint follows the claim shape
// in duragraph-spec/auth/jwt.yml.
//
// Layout:
//
//   - exchanger.go (this file): the [Exchanger] interface that abstracts the
//     goth call surface, plus a default [GothExchanger] that wraps gothic.
//     Tests substitute a stub.
//
//   - handler.go: the [Handler] struct, its constructor, and the four route
//     methods (Login, Callback, Logout, Refresh).
//
// The handler is intentionally NOT wired into cmd/server/main.go in this PR
// — that's a follow-up. Construction is exported so a future routing PR can
// call NewHandler from main.
package auth

import (
	"context"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"
)

// Exchanger abstracts the OAuth provider exchange so the callback decision-
// tree tests don't have to round-trip through real Google / GitHub. Tests
// substitute a stub returning a pre-built goth.User.
//
// The shape is "two methods, request-scoped" rather than "one method
// returning a URL" because gothic.BeginAuthHandler also writes the state
// cookie via Set-Cookie — we need to give it the live ResponseWriter, not
// just receive a URL string.
type Exchanger interface {
	// BeginAuth starts the OAuth flow for the given provider. On success the
	// implementation MUST 302-redirect the response (this is what
	// gothic.BeginAuthHandler does). The provider name is taken from the
	// route parameter so the caller doesn't have to mutate the request URL.
	BeginAuth(w http.ResponseWriter, r *http.Request, provider string) error

	// CompleteAuth completes the OAuth flow and returns the provider
	// userinfo. State validation happens inside gothic; on state mismatch
	// the underlying error surfaces and the handler reports 400.
	CompleteAuth(w http.ResponseWriter, r *http.Request, provider string) (goth.User, error)
}

// GothExchanger is the production [Exchanger] backed by github.com/markbates/goth.
// It bridges Echo's :provider path param to gothic's request-context-based
// provider lookup (gothic's URL parser is gorilla/mux-flavored and does NOT
// recognise Echo's :provider; without GetContextWithProvider gothic would
// fall back to the `provider=` query parameter, which we don't set).
type GothExchanger struct{}

// NewGothExchanger constructs the default Exchanger.
func NewGothExchanger() *GothExchanger { return &GothExchanger{} }

// BeginAuth implements [Exchanger].
//
// gothic.BeginAuthHandler:
//   - Generates a random state token.
//   - Persists it in gothic.Store (the cookie session — `_gothic_session`
//     by default, see duragraph-spec/auth/oauth.yml § state_csrf).
//   - 302-redirects to the provider's authorization endpoint with
//     `state=<token>` in the query string.
//
// We MUST inject the provider name via gothic.GetContextWithProvider before
// calling — gothic.GetProviderName looks for it in (1) request context, (2)
// URL query `provider=`, (3) gorilla mux vars. None of (2)/(3) apply to an
// Echo handler with a :provider path param.
func (g *GothExchanger) BeginAuth(w http.ResponseWriter, r *http.Request, provider string) error {
	r = gothic.GetContextWithProvider(r, provider)
	gothic.BeginAuthHandler(w, r)
	return nil
}

// CompleteAuth implements [Exchanger].
//
// gothic.CompleteUserAuth:
//   - Reads the state from the request and verifies it against the
//     `_gothic_session` cookie. Mismatches return an error.
//   - Exchanges the authorization code for an access token.
//   - Fetches the provider userinfo and returns it as goth.User.
//
// Same provider-context injection as BeginAuth.
func (g *GothExchanger) CompleteAuth(w http.ResponseWriter, r *http.Request, provider string) (goth.User, error) {
	r = gothic.GetContextWithProvider(r, provider)
	return gothic.CompleteUserAuth(w, r)
}

// BootstrapLocker abstracts the single-row bootstrap-lock claim used by the
// callback decision tree's bootstrap_first_user branch (per spec
// duragraph-spec/auth/oauth.yml § callback_flow.bootstrap_first_user.atomicity).
//
// TryClaim semantics:
//   - (true,  nil): this caller is the elected bootstrap-first-user.
//     Proceed to register the auto-admin and provision their tenant.
//   - (false, nil): another caller already won. Fall through to new_user.
//   - (false, err): unexpected DB error. Bubble up as 500.
//
// Pulling this out as an interface (mirroring [Exchanger]) keeps the
// bootstrap-lost case unit-testable: tests substitute an in-memory
// implementation rather than going through a real Postgres pool.
type BootstrapLocker interface {
	TryClaim(ctx context.Context) (bool, error)
}

// PoolBootstrapLocker is the production [BootstrapLocker] backed by the
// platform DB. It executes the canonical INSERT against
// platform.bootstrap_lock and translates SQLSTATE 23505 (unique_violation)
// into (false, nil) — the lock is taken; this caller lost.
type PoolBootstrapLocker struct {
	pool *pgxpool.Pool
}

// NewPoolBootstrapLocker constructs a [BootstrapLocker] against the given
// platform DB pool. The pool MUST be connected to the platform DB —
// platform.bootstrap_lock lives there and is a no-op anywhere else.
func NewPoolBootstrapLocker(pool *pgxpool.Pool) *PoolBootstrapLocker {
	return &PoolBootstrapLocker{pool: pool}
}

// TryClaim implements [BootstrapLocker].
func (l *PoolBootstrapLocker) TryClaim(ctx context.Context) (bool, error) {
	const q = `INSERT INTO platform.bootstrap_lock (id) VALUES (true)`
	_, err := l.pool.Exec(ctx, q)
	if err == nil {
		return true, nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return false, nil
	}
	return false, err
}
