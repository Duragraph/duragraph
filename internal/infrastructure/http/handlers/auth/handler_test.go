// Tests for the OAuth callback decision tree, logout CSRF, and refresh.
//
// Strategy:
//
//   - Mock repositories are hand-rolled (matching the rest of the engine
//     which doesn't use testify/mock). Each mock has a CountAll / Save /
//     GetByOAuth / GetByUserID closure so each test wires its own behavior.
//
//   - The OAuth provider exchange is stubbed via the [Exchanger] interface;
//     no real HTTP round-trip to Google / GitHub.
//
//   - The bootstrap-lock claim is abstracted behind [BootstrapLocker]; tests
//     pass a stub locker that returns canned (claimed, err) tuples.
//
//   - We do NOT exercise the bootstrap-WIN happy path in unit tests because
//     it calls migrator.ProvisionTenant, which requires a real Postgres;
//     that's covered by the integration tests in the postgres package. The
//     bootstrap-LOST case (this caller loses the race, falls through to
//     new_user) IS unit-tested here.

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/labstack/echo/v4"
	"github.com/markbates/goth"
	"github.com/markbates/goth/providers/github"
	"github.com/markbates/goth/providers/google"

	authpkg "github.com/duragraph/duragraph/internal/infrastructure/auth"

	"github.com/duragraph/duragraph/internal/domain/tenant"
	"github.com/duragraph/duragraph/internal/domain/user"
	pkgerrors "github.com/duragraph/duragraph/internal/pkg/errors"
)

// TestMain registers stub google + github providers with goth before the
// suite runs. Login/Callback now check goth.GetProvider up front and would
// otherwise reject every test that uses a "google" or "github" path param.
// The stub provider constructors don't make HTTP calls, so registration is
// cheap and pure; tests that exercise the not-registered branch snapshot
// + restore the global state via t.Cleanup.
func TestMain(m *testing.M) {
	goth.UseProviders(
		google.New("test-google-id", "test-google-secret", "http://test/api/auth/google/callback"),
		github.New("test-github-id", "test-github-secret", "http://test/api/auth/github/callback"),
	)
	os.Exit(m.Run())
}

// withClearedProviders snapshots goth's provider table, clears it for the
// caller, and restores it on test cleanup. Used by tests that need to
// observe behaviour when a provider is in supportedProviders but not
// registered with goth.
func withClearedProviders(t *testing.T) {
	t.Helper()
	saved := goth.GetProviders()
	t.Cleanup(func() {
		goth.ClearProviders()
		for _, p := range saved {
			goth.UseProviders(p)
		}
	})
	goth.ClearProviders()
}

// ---- shared test fixtures ------------------------------------------------

const (
	testJWTSecret = "oauth-handler-test-secret-32bytes!!"
	testBaseURL   = "https://platform.example.com"
)

// stubExchanger is a hand-rolled [Exchanger] returning a pre-built goth.User
// from CompleteAuth. BeginAuth writes a 302 to a sentinel URL so tests can
// assert it was reached without coupling to gothic internals.
type stubExchanger struct {
	user        goth.User
	completeErr error
	beginCalled bool
}

func (s *stubExchanger) BeginAuth(w http.ResponseWriter, r *http.Request, provider string) error {
	s.beginCalled = true
	w.Header().Set("Location", "https://provider.test/authorize?state=x")
	w.WriteHeader(http.StatusFound)
	return nil
}

func (s *stubExchanger) CompleteAuth(w http.ResponseWriter, r *http.Request, provider string) (goth.User, error) {
	if s.completeErr != nil {
		return goth.User{}, s.completeErr
	}
	return s.user, nil
}

// stubUserRepo + stubTenantRepo: behavior is fully driven by closures so
// each test only sets the responses it cares about.
type stubUserRepo struct {
	saveFn         func(ctx context.Context, u *user.User) error
	getByOAuthFn   func(ctx context.Context, provider, oauthID string) (*user.User, error)
	getByIDFn      func(ctx context.Context, id string) (*user.User, error)
	countAllFn     func(ctx context.Context) (int, error)
	listByStatusFn func(ctx context.Context, status user.Status, limit, offset int) ([]*user.User, error)
}

func (r *stubUserRepo) Save(ctx context.Context, u *user.User) error {
	if r.saveFn != nil {
		return r.saveFn(ctx, u)
	}
	return nil
}
func (r *stubUserRepo) GetByID(ctx context.Context, id string) (*user.User, error) {
	if r.getByIDFn != nil {
		return r.getByIDFn(ctx, id)
	}
	return nil, pkgerrors.NotFound("user", id)
}
func (r *stubUserRepo) GetByOAuth(ctx context.Context, provider, oauthID string) (*user.User, error) {
	if r.getByOAuthFn != nil {
		return r.getByOAuthFn(ctx, provider, oauthID)
	}
	return nil, pkgerrors.NotFound("user", provider+"/"+oauthID)
}
func (r *stubUserRepo) ListByStatus(ctx context.Context, status user.Status, limit, offset int) ([]*user.User, error) {
	if r.listByStatusFn != nil {
		return r.listByStatusFn(ctx, status, limit, offset)
	}
	return nil, nil
}
func (r *stubUserRepo) CountAll(ctx context.Context) (int, error) {
	if r.countAllFn != nil {
		return r.countAllFn(ctx)
	}
	return 0, nil
}

// stubLocker is a hand-rolled [BootstrapLocker]. Tests configure the
// canned response per case. Default zero-value returns (false, nil) —
// i.e. "another caller already won". Tests that simulate the WIN path
// must explicitly set claimed=true.
type stubLocker struct {
	claimed bool
	err     error
	calls   int
}

func (s *stubLocker) TryClaim(ctx context.Context) (bool, error) {
	s.calls++
	return s.claimed, s.err
}

type stubTenantRepo struct {
	saveFn         func(ctx context.Context, t *tenant.Tenant) error
	getByIDFn      func(ctx context.Context, id string) (*tenant.Tenant, error)
	getByUserIDFn  func(ctx context.Context, userID string) (*tenant.Tenant, error)
	listByStatusFn func(ctx context.Context, status tenant.Status, limit, offset int) ([]*tenant.Tenant, error)
}

func (r *stubTenantRepo) Save(ctx context.Context, t *tenant.Tenant) error {
	if r.saveFn != nil {
		return r.saveFn(ctx, t)
	}
	return nil
}
func (r *stubTenantRepo) GetByID(ctx context.Context, id string) (*tenant.Tenant, error) {
	if r.getByIDFn != nil {
		return r.getByIDFn(ctx, id)
	}
	return nil, pkgerrors.NotFound("tenant", id)
}
func (r *stubTenantRepo) GetByUserID(ctx context.Context, userID string) (*tenant.Tenant, error) {
	if r.getByUserIDFn != nil {
		return r.getByUserIDFn(ctx, userID)
	}
	return nil, pkgerrors.NotFound("tenant for user", userID)
}
func (r *stubTenantRepo) ListByStatus(ctx context.Context, status tenant.Status, limit, offset int) ([]*tenant.Tenant, error) {
	if r.listByStatusFn != nil {
		return r.listByStatusFn(ctx, status, limit, offset)
	}
	return nil, nil
}

// newTestHandler builds a Handler ready for unit testing.
//
// migrator stays nil — none of the unit tests exercise the bootstrap-WIN
// branch (that requires a real Postgres for ProvisionTenant; covered by
// integration tests). Construct the Handler struct directly rather than
// going through NewHandler so we can leave migrator nil.
func newTestHandler(t *testing.T, opts ...func(*Handler)) *Handler {
	t.Helper()

	verifier, err := authpkg.NewVerifier([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}

	h := &Handler{
		userRepo:   &stubUserRepo{},
		tenantRepo: &stubTenantRepo{},
		migrator:   nil,
		verifier:   verifier,
		exchanger:  &stubExchanger{},
		// Default: lost the race / not bootstrap path. Tests that
		// trigger bootstrap-WIN override with claimed=true (and
		// integration tests cover the actual Postgres path).
		locker: &stubLocker{},
		cfg: Config{
			SessionTTL:   24 * time.Hour,
			CookieDomain: "",
			CookieSecure: false,
			BaseURL:      testBaseURL,
			JWTSecret:    []byte(testJWTSecret),
		},
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

func newEchoCtx(t *testing.T, method, target string, body string) (echo.Context, *httptest.ResponseRecorder, *echo.Echo) {
	t.Helper()
	e := echo.New()
	rec := httptest.NewRecorder()
	var bodyReader *strings.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}
	var req *http.Request
	if bodyReader == nil {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, bodyReader)
	}
	c := e.NewContext(req, rec)
	return c, rec, e
}

// pathParam attaches an Echo path-param value (since httptest.NewRequest
// alone doesn't populate the route params).
func pathParam(c echo.Context, name, value string) {
	c.SetParamNames(name)
	c.SetParamValues(value)
}

// ---- decision-tree branches ----------------------------------------------

// approved → JWT with role + tenant_id, redirect /.
func TestCallback_ExistingUser_Approved(t *testing.T) {
	approvedUser := user.ReconstructFromData(user.UserData{
		ID:            "user-1",
		Email:         "alice@example.com",
		OAuthProvider: "google",
		OAuthID:       "google-sub-1",
		Role:          string(user.RoleAdmin),
		Status:        string(user.StatusApproved),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	})
	approvedTenant := tenant.ReconstructFromData(tenant.TenantData{
		ID:        "11111111-1111-1111-1111-111111111111",
		UserID:    "user-1",
		DBName:    "tenant_11111111111111111111111111111111",
		Status:    string(tenant.StatusApproved),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})

	h := newTestHandler(t, func(h *Handler) {
		h.userRepo = &stubUserRepo{
			getByOAuthFn: func(ctx context.Context, provider, oauthID string) (*user.User, error) {
				return approvedUser, nil
			},
		}
		h.tenantRepo = &stubTenantRepo{
			getByUserIDFn: func(ctx context.Context, userID string) (*tenant.Tenant, error) {
				return approvedTenant, nil
			},
		}
		h.exchanger = &stubExchanger{user: goth.User{
			Provider: "google", UserID: "google-sub-1", Email: "alice@example.com",
		}}
	})

	c, rec, _ := newEchoCtx(t, http.MethodGet, "/api/auth/google/callback?code=x&state=y", "")
	pathParam(c, "provider", "google")

	if err := h.Callback(c); err != nil {
		t.Fatalf("Callback: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Errorf("expected 302, got %d (body=%s)", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != dashboardRoot {
		t.Errorf("expected redirect to %s, got %s", dashboardRoot, loc)
	}

	// Verify the cookie was set with a valid token containing tenant_id.
	cookie := findCookie(rec.Result().Cookies(), SessionCookieName)
	if cookie == nil {
		t.Fatal("expected duragraph_session cookie")
	}
	verified, err := authpkg.VerifyJWT([]byte(testJWTSecret), cookie.Value)
	if err != nil {
		t.Fatalf("verify cookie token: %v", err)
	}
	if verified.UserID != "user-1" {
		t.Errorf("token user_id: want user-1, got %q", verified.UserID)
	}
	if verified.TenantID != approvedTenant.ID() {
		t.Errorf("token tenant_id: want %q, got %q", approvedTenant.ID(), verified.TenantID)
	}
	if verified.Role != string(user.RoleAdmin) {
		t.Errorf("token role: want admin, got %q", verified.Role)
	}
}

// pending → JWT with empty tenant_id, redirect /awaiting-approval.
func TestCallback_ExistingUser_Pending(t *testing.T) {
	pendingUser := user.ReconstructFromData(user.UserData{
		ID:            "user-2",
		Email:         "bob@example.com",
		OAuthProvider: "github",
		OAuthID:       "12345",
		Role:          string(user.RoleUser),
		Status:        string(user.StatusPending),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	})

	h := newTestHandler(t, func(h *Handler) {
		h.userRepo = &stubUserRepo{
			getByOAuthFn: func(ctx context.Context, provider, oauthID string) (*user.User, error) {
				return pendingUser, nil
			},
		}
		h.exchanger = &stubExchanger{user: goth.User{
			Provider: "github", UserID: "12345", Email: "bob@example.com",
		}}
	})

	c, rec, _ := newEchoCtx(t, http.MethodGet, "/api/auth/github/callback?code=x&state=y", "")
	pathParam(c, "provider", "github")

	if err := h.Callback(c); err != nil {
		t.Fatalf("Callback: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Errorf("expected 302, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != awaitingApprovalRoute {
		t.Errorf("expected redirect to %s, got %s", awaitingApprovalRoute, loc)
	}
	cookie := findCookie(rec.Result().Cookies(), SessionCookieName)
	if cookie == nil {
		t.Fatal("expected duragraph_session cookie even for pending user")
	}
	verified, err := authpkg.VerifyJWT([]byte(testJWTSecret), cookie.Value)
	if err != nil {
		t.Fatalf("verify cookie token: %v", err)
	}
	if verified.TenantID != "" {
		t.Errorf("pending user tenant_id should be empty, got %q", verified.TenantID)
	}
	if verified.Role != string(user.RoleUser) {
		t.Errorf("pending user role: want user, got %q", verified.Role)
	}
}

// suspended → no JWT, no cookie, redirect /suspended.
func TestCallback_ExistingUser_Suspended(t *testing.T) {
	suspendedUser := user.ReconstructFromData(user.UserData{
		ID:            "user-3",
		Email:         "carol@example.com",
		OAuthProvider: "google",
		OAuthID:       "g3",
		Role:          string(user.RoleUser),
		Status:        string(user.StatusSuspended),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	})

	h := newTestHandler(t, func(h *Handler) {
		h.userRepo = &stubUserRepo{
			getByOAuthFn: func(ctx context.Context, provider, oauthID string) (*user.User, error) {
				return suspendedUser, nil
			},
		}
		h.exchanger = &stubExchanger{user: goth.User{
			Provider: "google", UserID: "g3", Email: "carol@example.com",
		}}
	})

	c, rec, _ := newEchoCtx(t, http.MethodGet, "/api/auth/google/callback?code=x&state=y", "")
	pathParam(c, "provider", "google")

	if err := h.Callback(c); err != nil {
		t.Fatalf("Callback: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Errorf("expected 302, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != suspendedRoute {
		t.Errorf("expected redirect to %s, got %s", suspendedRoute, loc)
	}
	if findCookie(rec.Result().Cookies(), SessionCookieName) != nil {
		t.Error("suspended user must NOT receive a session cookie")
	}
}

// new_user (count > 0, GetByOAuth returns NotFound, Save succeeds) →
// pending JWT, redirect /awaiting-approval.
func TestCallback_NewUser_Success(t *testing.T) {
	var savedUser *user.User
	h := newTestHandler(t, func(h *Handler) {
		h.userRepo = &stubUserRepo{
			getByOAuthFn: func(ctx context.Context, provider, oauthID string) (*user.User, error) {
				return nil, pkgerrors.NotFound("user", provider+"/"+oauthID)
			},
			countAllFn: func(ctx context.Context) (int, error) {
				return 1, nil // not the first user
			},
			saveFn: func(ctx context.Context, u *user.User) error {
				savedUser = u
				return nil
			},
		}
		h.exchanger = &stubExchanger{user: goth.User{
			Provider: "google", UserID: "g-new", Email: "dave@example.com",
		}}
	})

	c, rec, _ := newEchoCtx(t, http.MethodGet, "/api/auth/google/callback?code=x&state=y", "")
	pathParam(c, "provider", "google")

	if err := h.Callback(c); err != nil {
		t.Fatalf("Callback: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Errorf("expected 302, got %d (body=%s)", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != awaitingApprovalRoute {
		t.Errorf("expected redirect to %s, got %s", awaitingApprovalRoute, loc)
	}
	if savedUser == nil {
		t.Fatal("expected userRepo.Save to have been called")
	}
	if savedUser.Status() != user.StatusPending {
		t.Errorf("new user status: want pending, got %s", savedUser.Status())
	}
	if savedUser.Role() != user.RoleUser {
		t.Errorf("new user role: want user, got %s", savedUser.Role())
	}

	// Cookie carries a pending JWT.
	cookie := findCookie(rec.Result().Cookies(), SessionCookieName)
	if cookie == nil {
		t.Fatal("expected duragraph_session cookie")
	}
	verified, err := authpkg.VerifyJWT([]byte(testJWTSecret), cookie.Value)
	if err != nil {
		t.Fatalf("verify cookie token: %v", err)
	}
	if verified.TenantID != "" {
		t.Errorf("new user tenant_id: want empty, got %q", verified.TenantID)
	}
}

// new_user race: Save returns SQLSTATE 23505 → handler re-fetches via
// GetByOAuth and treats the row as existing. The "winner" is a pending
// user, so the loser also redirects to /awaiting-approval.
func TestCallback_NewUser_RaceLoser(t *testing.T) {
	winnerRow := user.ReconstructFromData(user.UserData{
		ID:            "user-winner",
		Email:         "race@example.com",
		OAuthProvider: "google",
		OAuthID:       "race-id",
		Role:          string(user.RoleUser),
		Status:        string(user.StatusPending),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	})

	getByOAuthCalls := 0
	h := newTestHandler(t, func(h *Handler) {
		h.userRepo = &stubUserRepo{
			getByOAuthFn: func(ctx context.Context, provider, oauthID string) (*user.User, error) {
				getByOAuthCalls++
				if getByOAuthCalls == 1 {
					// First lookup: not found (mirrors the pre-INSERT probe).
					return nil, pkgerrors.NotFound("user", provider+"/"+oauthID)
				}
				// Second lookup (post-23505): the winner's row is now visible.
				return winnerRow, nil
			},
			countAllFn: func(ctx context.Context) (int, error) { return 1, nil },
			saveFn: func(ctx context.Context, u *user.User) error {
				// Simulate the unique-violation the loser of the race observes.
				return pkgerrors.Internal("failed to insert user", &pgconn.PgError{Code: "23505"})
			},
		}
		h.exchanger = &stubExchanger{user: goth.User{
			Provider: "google", UserID: "race-id", Email: "race@example.com",
		}}
	})

	c, rec, _ := newEchoCtx(t, http.MethodGet, "/api/auth/google/callback?code=x&state=y", "")
	pathParam(c, "provider", "google")

	if err := h.Callback(c); err != nil {
		t.Fatalf("Callback: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Errorf("expected 302, got %d (body=%s)", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != awaitingApprovalRoute {
		t.Errorf("expected redirect to %s, got %s", awaitingApprovalRoute, loc)
	}
	if getByOAuthCalls != 2 {
		t.Errorf("expected 2 GetByOAuth calls (probe + race recovery), got %d", getByOAuthCalls)
	}
}

// bootstrap-LOST: count==0 (looks like the bootstrap branch) but the locker
// reports another caller already won → fall through to new_user path with
// a pending JWT and /awaiting-approval redirect.
//
// This proves the spec's "concurrent first signup → loser becomes pending"
// semantics from § callback_flow.bootstrap_first_user.atomicity.
func TestCallback_BootstrapLost_FallsThroughToNewUser(t *testing.T) {
	var savedUser *user.User
	locker := &stubLocker{claimed: false} // we lose the race
	h := newTestHandler(t, func(h *Handler) {
		h.locker = locker
		h.userRepo = &stubUserRepo{
			getByOAuthFn: func(ctx context.Context, provider, oauthID string) (*user.User, error) {
				return nil, pkgerrors.NotFound("user", provider+"/"+oauthID)
			},
			countAllFn: func(ctx context.Context) (int, error) { return 0, nil },
			saveFn: func(ctx context.Context, u *user.User) error {
				savedUser = u
				return nil
			},
		}
		h.exchanger = &stubExchanger{user: goth.User{
			Provider: "google", UserID: "g-loser", Email: "loser@example.com",
		}}
	})

	c, rec, _ := newEchoCtx(t, http.MethodGet, "/api/auth/google/callback?code=x&state=y", "")
	pathParam(c, "provider", "google")

	if err := h.Callback(c); err != nil {
		t.Fatalf("Callback: %v", err)
	}
	if locker.calls != 1 {
		t.Errorf("expected locker.TryClaim to be called exactly once, got %d", locker.calls)
	}
	if rec.Code != http.StatusFound {
		t.Errorf("expected 302, got %d (body=%s)", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != awaitingApprovalRoute {
		t.Errorf("expected redirect to %s (loser becomes pending), got %s", awaitingApprovalRoute, loc)
	}
	if savedUser == nil {
		t.Fatal("expected userRepo.Save to have been called for the loser")
	}
	if savedUser.Status() != user.StatusPending {
		t.Errorf("loser status: want pending, got %s", savedUser.Status())
	}
	if savedUser.Role() != user.RoleUser {
		t.Errorf("loser role: want user (NOT admin), got %s", savedUser.Role())
	}
}

// bootstrap-locker-error: count==0 + locker returns a non-23505 error →
// 500. Distinguishes "lock already taken" from "DB connection broke".
func TestCallback_BootstrapLockerError(t *testing.T) {
	h := newTestHandler(t, func(h *Handler) {
		h.locker = &stubLocker{err: errors.New("connection refused")}
		h.userRepo = &stubUserRepo{
			getByOAuthFn: func(ctx context.Context, provider, oauthID string) (*user.User, error) {
				return nil, pkgerrors.NotFound("user", provider+"/"+oauthID)
			},
			countAllFn: func(ctx context.Context) (int, error) { return 0, nil },
		}
		h.exchanger = &stubExchanger{user: goth.User{
			Provider: "google", UserID: "g-error", Email: "x@example.com",
		}}
	})

	c, rec, _ := newEchoCtx(t, http.MethodGet, "/api/auth/google/callback?code=x&state=y", "")
	pathParam(c, "provider", "google")

	if err := h.Callback(c); err != nil {
		t.Fatalf("Callback: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d (body=%s)", rec.Code, rec.Body.String())
	}
}

// ---- error / edge cases --------------------------------------------------

// Unknown provider on /login.
func TestLogin_UnknownProvider(t *testing.T) {
	h := newTestHandler(t)
	c, rec, _ := newEchoCtx(t, http.MethodGet, "/api/auth/twitter/login", "")
	pathParam(c, "provider", "twitter")

	if err := h.Login(c); err != nil {
		t.Fatalf("Login: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

// Unknown provider on /callback.
func TestCallback_UnknownProvider(t *testing.T) {
	h := newTestHandler(t)
	c, rec, _ := newEchoCtx(t, http.MethodGet, "/api/auth/twitter/callback?code=x", "")
	pathParam(c, "provider", "twitter")

	if err := h.Callback(c); err != nil {
		t.Fatalf("Callback: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

// State mismatch from gothic → 400 with state_mismatch code.
func TestCallback_StateMismatch(t *testing.T) {
	h := newTestHandler(t, func(h *Handler) {
		h.exchanger = &stubExchanger{
			completeErr: errors.New("state token mismatch"),
		}
	})
	c, rec, _ := newEchoCtx(t, http.MethodGet, "/api/auth/google/callback?code=x&state=y", "")
	pathParam(c, "provider", "google")

	if err := h.Callback(c); err != nil {
		t.Fatalf("Callback: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d (body=%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "state_mismatch") {
		t.Errorf("expected state_mismatch error code in body, got %s", rec.Body.String())
	}
}

// Provider exchange failure (non-state) → 502.
func TestCallback_ProviderExchangeFailed(t *testing.T) {
	h := newTestHandler(t, func(h *Handler) {
		h.exchanger = &stubExchanger{
			completeErr: errors.New("network unreachable"),
		}
	})
	c, rec, _ := newEchoCtx(t, http.MethodGet, "/api/auth/google/callback?code=x&state=y", "")
	pathParam(c, "provider", "google")

	if err := h.Callback(c); err != nil {
		t.Fatalf("Callback: %v", err)
	}
	if rec.Code != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", rec.Code)
	}
}

// Empty email (e.g. GitHub user with no verified primary email and the
// fallback also returning nothing) → 400 no_verified_email.
func TestCallback_NoVerifiedEmail(t *testing.T) {
	h := newTestHandler(t, func(h *Handler) {
		h.exchanger = &stubExchanger{user: goth.User{
			Provider: "github", UserID: "12345", Email: "", // empty
		}}
	})
	c, rec, _ := newEchoCtx(t, http.MethodGet, "/api/auth/github/callback?code=x&state=y", "")
	pathParam(c, "provider", "github")

	if err := h.Callback(c); err != nil {
		t.Fatalf("Callback: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "no_verified_email") {
		t.Errorf("expected no_verified_email code in body, got %s", rec.Body.String())
	}
}

// /login with a valid provider hands off to the exchanger and we observe
// the 302 it wrote.
func TestLogin_HandsOffToExchanger(t *testing.T) {
	stub := &stubExchanger{}
	h := newTestHandler(t, func(h *Handler) {
		h.exchanger = stub
	})
	c, rec, _ := newEchoCtx(t, http.MethodGet, "/api/auth/google/login", "")
	pathParam(c, "provider", "google")

	if err := h.Login(c); err != nil {
		t.Fatalf("Login: %v", err)
	}
	if !stub.beginCalled {
		t.Error("expected stubExchanger.BeginAuth to have been called")
	}
	if rec.Code != http.StatusFound {
		t.Errorf("expected 302, got %d", rec.Code)
	}
}

// ---- logout --------------------------------------------------------------

// Bearer logout: no Origin/Referer needed → 204 + cleared cookie.
func TestLogout_BearerSkipsCSRF(t *testing.T) {
	h := newTestHandler(t)
	c, rec, _ := newEchoCtx(t, http.MethodPost, "/api/auth/logout", "")
	c.Request().Header.Set("Authorization", "Bearer xyz")
	// No Origin / Referer.

	if err := h.Logout(c); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d (body=%s)", rec.Code, rec.Body.String())
	}
	cookie := findCookie(rec.Result().Cookies(), SessionCookieName)
	if cookie == nil {
		t.Fatal("expected a Set-Cookie clearing the session cookie")
	}
	if cookie.MaxAge != -1 {
		t.Errorf("expected Max-Age=-1 (delete), got %d", cookie.MaxAge)
	}
}

// Cookie logout with matching Origin → 204.
func TestLogout_CookieMatchingOrigin(t *testing.T) {
	h := newTestHandler(t)
	c, rec, _ := newEchoCtx(t, http.MethodPost, "/api/auth/logout", "")
	// Cookie auth (no Authorization header), Origin matches BaseURL.
	c.Request().Header.Set("Origin", testBaseURL)

	if err := h.Logout(c); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}
}

// Cookie logout with mismatched Origin → 403.
func TestLogout_CookieMismatchedOrigin(t *testing.T) {
	h := newTestHandler(t)
	c, rec, _ := newEchoCtx(t, http.MethodPost, "/api/auth/logout", "")
	c.Request().Header.Set("Origin", "https://evil.example.com")

	if err := h.Logout(c); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

// Cookie logout with no Origin and no Referer → 403.
func TestLogout_CookieNoOriginOrReferer(t *testing.T) {
	h := newTestHandler(t)
	c, rec, _ := newEchoCtx(t, http.MethodPost, "/api/auth/logout", "")
	// No Origin, no Referer, no Authorization.

	if err := h.Logout(c); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

// Referer fallback when Origin missing.
func TestLogout_CookieRefererFallback(t *testing.T) {
	h := newTestHandler(t)
	c, rec, _ := newEchoCtx(t, http.MethodPost, "/api/auth/logout", "")
	c.Request().Header.Set("Referer", testBaseURL+"/dashboard")

	if err := h.Logout(c); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}
}

// ---- refresh -------------------------------------------------------------

// Valid bearer token → new token returned in JSON.
//
// Asserts the response body's exp is exactly equal to the freshly-issued
// token's exp claim (no clock-skew off-by-one between IssueJWT's internal
// time.Now() and a separately-computed time.Now() in the handler).
func TestRefresh_Valid(t *testing.T) {
	h := newTestHandler(t)
	// Mint a current token.
	tok, err := authpkg.IssueJWT(
		[]byte(testJWTSecret),
		"user-1", "alice@example.com", string(user.RoleUser), "tenant-1",
		1*time.Hour,
	)
	if err != nil {
		t.Fatalf("IssueJWT: %v", err)
	}

	c, rec, _ := newEchoCtx(t, http.MethodPost, "/api/auth/refresh", "")
	c.Request().Header.Set("Authorization", "Bearer "+tok)

	if err := h.Refresh(c); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d (body=%s)", rec.Code, rec.Body.String())
	}

	var body struct {
		Token string `json:"token"`
		Exp   int64  `json:"exp"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v (body=%s)", err, rec.Body.String())
	}
	if body.Token == "" {
		t.Errorf("expected non-empty token, got %q", body.Token)
	}
	if body.Exp == 0 {
		t.Errorf("expected non-zero exp, got %d", body.Exp)
	}

	// The exp in the body must match the freshly-issued JWT's exp claim
	// exactly. Any drift means we're computing two different time.Now()s.
	verified, err := authpkg.VerifyJWT([]byte(testJWTSecret), body.Token)
	if err != nil {
		t.Fatalf("verify refreshed token: %v", err)
	}
	if verified.ExpiresAt == nil {
		t.Fatal("refreshed token has no ExpiresAt claim")
	}
	if got, want := body.Exp, verified.ExpiresAt.Unix(); got != want {
		t.Errorf("body exp does not match token exp: body=%d token=%d (drift=%d)", got, want, got-want)
	}
}

// No Authorization header → 401.
func TestRefresh_NoBearer(t *testing.T) {
	h := newTestHandler(t)
	c, rec, _ := newEchoCtx(t, http.MethodPost, "/api/auth/refresh", "")
	// No Authorization header.

	if err := h.Refresh(c); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

// Expired bearer token → 401.
func TestRefresh_ExpiredBearer(t *testing.T) {
	h := newTestHandler(t)
	// Mint a token that's already expired. IssueJWT requires positive
	// ttl; we sidestep that by using a tiny ttl and waiting briefly.
	tok, err := authpkg.IssueJWT(
		[]byte(testJWTSecret),
		"user-1", "alice@example.com", string(user.RoleUser), "tenant-1",
		1*time.Nanosecond,
	)
	if err != nil {
		t.Fatalf("IssueJWT: %v", err)
	}
	// Sleep one ms — the JWT exp is now in the past.
	time.Sleep(2 * time.Millisecond)

	c, rec, _ := newEchoCtx(t, http.MethodPost, "/api/auth/refresh", "")
	c.Request().Header.Set("Authorization", "Bearer "+tok)

	if err := h.Refresh(c); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for expired token, got %d", rec.Code)
	}
}

// ---- helpers --------------------------------------------------------------

func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, ck := range cookies {
		if ck.Name == name {
			return ck
		}
	}
	return nil
}

// Smoke test for originMatchesBaseURL — the helper itself is also exercised
// indirectly via the logout tests, but we cover the URL parsing edge cases
// here too (empty BaseURL → permissive, malformed Origin → reject, etc.).
func TestOriginMatchesBaseURL(t *testing.T) {
	cases := []struct {
		name    string
		baseURL string
		origin  string
		referer string
		want    bool
	}{
		// originMatchesBaseURL is now fail-closed when BaseURL is unparseable
		// or empty (NewHandler rejects empty BaseURL, but the helper itself
		// must remain safe under direct struct construction).
		{"empty base url rejects", "", "https://anywhere.test", "", false},
		{"matching origin", testBaseURL, testBaseURL, "", true},
		{"mismatched origin", testBaseURL, "https://other.example", "", false},
		{"no origin no referer", testBaseURL, "", "", false},
		{"matching referer", testBaseURL, "", testBaseURL + "/foo", true},
		{"mismatched referer", testBaseURL, "", "https://other.example/x", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &Handler{cfg: Config{BaseURL: tc.baseURL}}
			req := httptest.NewRequest(http.MethodPost, "/x", nil)
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.referer != "" {
				req.Header.Set("Referer", tc.referer)
			}
			got := h.originMatchesBaseURL(req)
			if got != tc.want {
				t.Errorf("origin=%q referer=%q: want %v got %v", tc.origin, tc.referer, tc.want, got)
			}
		})
	}
}

// Ensure the URL-parsing helper used by originMatchesBaseURL handles
// bizarre inputs without panicking.
func TestOriginMatchesBaseURL_BadInputs(t *testing.T) {
	h := &Handler{cfg: Config{BaseURL: "::not-a-url"}}
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	req.Header.Set("Origin", "https://x.test")
	if h.originMatchesBaseURL(req) {
		t.Error("malformed BaseURL should reject all origins")
	}

	// Also ensure url.Parse doesn't blow up on weird Origin strings — even
	// browsers can occasionally send "null" for sandboxed iframes.
	h = &Handler{cfg: Config{BaseURL: testBaseURL}}
	req = httptest.NewRequest(http.MethodPost, "/x", nil)
	req.Header.Set("Origin", "null")
	if h.originMatchesBaseURL(req) {
		t.Error("Origin=\"null\" should not match a real BaseURL")
	}
}

// Sanity-check: the local errorBody helper produces a stable JSON shape.
// Tests against the wire format matter because the spec calls out specific
// error codes (state_mismatch, no_verified_email, etc.).
func TestErrorBody_Shape(t *testing.T) {
	body := errorBody("foo", "bar")
	if body["error"] != "foo" || body["message"] != "bar" {
		t.Errorf("errorBody shape changed: %+v", body)
	}
}

// Sanity-check that our isUniqueViolation helper recognises the wrapped
// pgconn.PgError chain produced by UserRepository.Save.
func TestIsUniqueViolation(t *testing.T) {
	wrapped := pkgerrors.Internal("failed to insert user", &pgconn.PgError{Code: "23505"})
	if !isUniqueViolation(wrapped) {
		t.Error("expected wrapped 23505 to be detected as unique violation")
	}

	notUnique := pkgerrors.Internal("failed to insert user", &pgconn.PgError{Code: "23502"})
	if isUniqueViolation(notUnique) {
		t.Error("23502 should NOT match")
	}

	plain := errors.New("not a pg error")
	if isUniqueViolation(plain) {
		t.Error("plain error should NOT match")
	}
}

// Sanity-check on the constructor's defensive arg validation.
//
// Each case zeroes one required arg at a time and asserts an error. The
// migrator nil-check is exercised even though we use nil migrators in the
// other unit tests (those skip NewHandler and build the struct directly).
func TestNewHandler_RequiredArgs(t *testing.T) {
	verifier, _ := authpkg.NewVerifier([]byte(testJWTSecret))
	cfg := Config{
		SessionTTL: 24 * time.Hour,
		BaseURL:    testBaseURL,
		JWTSecret:  []byte(testJWTSecret),
	}

	if _, err := NewHandler(nil, &stubTenantRepo{}, nil, verifier, &stubExchanger{}, &stubLocker{}, cfg); err == nil {
		t.Error("expected error when userRepo is nil")
	}
	if _, err := NewHandler(&stubUserRepo{}, nil, nil, verifier, &stubExchanger{}, &stubLocker{}, cfg); err == nil {
		t.Error("expected error when tenantRepo is nil")
	}
	if _, err := NewHandler(&stubUserRepo{}, &stubTenantRepo{}, nil, verifier, &stubExchanger{}, &stubLocker{}, cfg); err == nil {
		t.Error("expected error when migrator is nil")
	}

	// Empty BaseURL must be rejected — accepting it would silently disable
	// the cookie-logout CSRF defence (originMatchesBaseURL is fail-closed
	// on an unparseable BaseURL).
	emptyBase := cfg
	emptyBase.BaseURL = ""
	if _, err := NewHandler(&stubUserRepo{}, &stubTenantRepo{}, nil, verifier, &stubExchanger{}, &stubLocker{}, emptyBase); err == nil {
		t.Error("expected error when BaseURL is empty")
	}
}

// /login when the provider is in supportedProviders but not registered
// with goth (e.g. OAUTH_GOOGLE_CLIENT_ID was empty in env, so
// ConfigureProviders skipped it) → 400 unknown_provider, NOT 502.
func TestLogin_ProviderNotRegistered(t *testing.T) {
	withClearedProviders(t)

	h := newTestHandler(t)
	c, rec, _ := newEchoCtx(t, http.MethodGet, "/api/auth/google/login", "")
	pathParam(c, "provider", "google")

	if err := h.Login(c); err != nil {
		t.Fatalf("Login: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 when provider not registered, got %d (body=%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "unknown_provider") {
		t.Errorf("expected unknown_provider error code, got %s", rec.Body.String())
	}
}

// /callback when the provider is in supportedProviders but not registered
// with goth → 400 unknown_provider, NOT 502 provider_exchange_failed.
func TestCallback_ProviderNotRegistered(t *testing.T) {
	withClearedProviders(t)

	h := newTestHandler(t)
	c, rec, _ := newEchoCtx(t, http.MethodGet, "/api/auth/github/callback?code=x&state=y", "")
	pathParam(c, "provider", "github")

	if err := h.Callback(c); err != nil {
		t.Fatalf("Callback: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 when provider not registered, got %d (body=%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "unknown_provider") {
		t.Errorf("expected unknown_provider error code, got %s", rec.Body.String())
	}
}

// Verify the BaseURL-parse round-trip used by originMatchesBaseURL — we
// rely on url.URL{Scheme,Host} comparisons not on string equality.
func TestBaseURLOriginExtraction(t *testing.T) {
	cases := []string{testBaseURL, "http://localhost:8081"}
	for _, c := range cases {
		u, err := url.Parse(c)
		if err != nil {
			t.Fatalf("parse %q: %v", c, err)
		}
		if u.Scheme == "" || u.Host == "" {
			t.Errorf("%q parsed without scheme/host", c)
		}
	}
}
