// Tests for the /api/platform/me handler. Mirrors the hand-rolled-stub
// pattern from internal/infrastructure/http/handlers/auth (no testify):
// each test wires a stubUserRepo / stubTenantRepo with the closure
// behaviour it cares about and seeds the request context's
// `platform.user_id` directly (matching admin/handler_test.go's pattern,
// since TenantMiddleware doesn't run in unit tests).

package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/duragraph/duragraph/internal/domain/tenant"
	"github.com/duragraph/duragraph/internal/domain/user"
	pkgerrors "github.com/duragraph/duragraph/internal/pkg/errors"
)

// ctxKeyPlatformUserID mirrors middleware.ctxKeyPlatformUserID. The
// middleware accessor is `UserIDFromCtx` (read-only) and the writer
// `withUserID` is unexported — tests in this package have to seed the
// raw key, exactly the pattern admin/handler_test.go uses. Keep the
// constant in sync with internal/infrastructure/http/middleware/ctxkeys.go.
const ctxKeyPlatformUserID = "platform.user_id"

// ---- stubs ---------------------------------------------------------------

type stubUserRepo struct {
	getByIDFn func(ctx context.Context, id string) (*user.User, error)
}

func (r *stubUserRepo) Save(_ context.Context, _ *user.User) error { return nil }
func (r *stubUserRepo) GetByID(ctx context.Context, id string) (*user.User, error) {
	if r.getByIDFn != nil {
		return r.getByIDFn(ctx, id)
	}
	return nil, pkgerrors.NotFound("user", id)
}
func (r *stubUserRepo) GetByOAuth(_ context.Context, provider, oauthID string) (*user.User, error) {
	return nil, pkgerrors.NotFound("user", provider+"/"+oauthID)
}
func (r *stubUserRepo) ListByStatus(_ context.Context, _ user.Status, _, _ int) ([]*user.User, error) {
	return nil, nil
}
func (r *stubUserRepo) List(_ context.Context, _ *user.Status, _, _ int) ([]*user.User, error) {
	return nil, nil
}
func (r *stubUserRepo) CountByStatus(_ context.Context, _ *user.Status) (int, error) {
	return 0, nil
}
func (r *stubUserRepo) CountAll(_ context.Context) (int, error) { return 0, nil }

type stubTenantRepo struct {
	getByUserIDFn func(ctx context.Context, userID string) (*tenant.Tenant, error)
}

func (r *stubTenantRepo) Save(_ context.Context, _ *tenant.Tenant) error { return nil }
func (r *stubTenantRepo) GetByID(_ context.Context, id string) (*tenant.Tenant, error) {
	return nil, pkgerrors.NotFound("tenant", id)
}
func (r *stubTenantRepo) GetByUserID(ctx context.Context, userID string) (*tenant.Tenant, error) {
	if r.getByUserIDFn != nil {
		return r.getByUserIDFn(ctx, userID)
	}
	return nil, pkgerrors.NotFound("tenant for user", userID)
}
func (r *stubTenantRepo) ListByStatus(_ context.Context, _ tenant.Status, _, _ int) ([]*tenant.Tenant, error) {
	return nil, nil
}

// ---- helpers -------------------------------------------------------------

func newCtx(t *testing.T, userID string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/platform/me", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if userID != "" {
		c.Set(ctxKeyPlatformUserID, userID)
	}
	return c, rec
}

func decodeMe(t *testing.T, rec *httptest.ResponseRecorder) MeResponse {
	t.Helper()
	var resp MeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal MeResponse: %v (body=%s)", err, rec.Body.String())
	}
	return resp
}

// ---- tests ---------------------------------------------------------------

// 401 when no user_id is present in the request context. Defence-in-
// depth — TenantMiddleware should always set it; absence means the
// route was mis-wired or token verification was bypassed.
func TestMe_NoUserID_401(t *testing.T) {
	h := NewHandler(&stubUserRepo{}, &stubTenantRepo{})

	c, rec := newCtx(t, "")
	if err := h.Me(c); err != nil {
		t.Fatalf("Me: %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d (body=%s)", rec.Code, rec.Body.String())
	}
}

// 401 when the user_id refers to a row that no longer exists (e.g.
// admin deleted the user mid-session). Treat as logged-out per the
// spec rather than 404 — the dashboard should clear its cookie.
func TestMe_UserNotFound_401(t *testing.T) {
	h := NewHandler(
		&stubUserRepo{
			getByIDFn: func(_ context.Context, _ string) (*user.User, error) {
				return nil, pkgerrors.NotFound("user", "missing")
			},
		},
		&stubTenantRepo{},
	)

	c, rec := newCtx(t, "missing")
	if err := h.Me(c); err != nil {
		t.Fatalf("Me: %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d (body=%s)", rec.Code, rec.Body.String())
	}
}

// 500 on unexpected user-repo errors (DB connection failure, etc.).
// Distinguished from 401-NotFound because the request is otherwise
// well-formed; surfacing 500 helps the dashboard distinguish "your
// session is bad, log out" from "the platform is having a bad day".
func TestMe_UserRepoError_500(t *testing.T) {
	h := NewHandler(
		&stubUserRepo{
			getByIDFn: func(_ context.Context, _ string) (*user.User, error) {
				return nil, errors.New("db dead")
			},
		},
		&stubTenantRepo{},
	)

	c, rec := newCtx(t, "user-1")
	if err := h.Me(c); err != nil {
		t.Fatalf("Me: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d (body=%s)", rec.Code, rec.Body.String())
	}
}

// Pending user → 200 with tenant_id=null. The tenant repo is NOT
// consulted because the user is pre-approval.
func TestMe_PendingUser_200_NullTenant(t *testing.T) {
	pending := user.ReconstructFromData(user.UserData{
		ID:            "user-pending",
		Email:         "alice@example.com",
		OAuthProvider: "google",
		OAuthID:       "google-sub-1",
		Role:          string(user.RoleUser),
		Status:        string(user.StatusPending),
		CreatedAt:     time.Now().UTC().Truncate(time.Second),
		UpdatedAt:     time.Now().UTC().Truncate(time.Second),
	})

	tenantCalls := 0
	h := NewHandler(
		&stubUserRepo{
			getByIDFn: func(_ context.Context, _ string) (*user.User, error) {
				return pending, nil
			},
		},
		&stubTenantRepo{
			getByUserIDFn: func(_ context.Context, _ string) (*tenant.Tenant, error) {
				tenantCalls++
				return nil, pkgerrors.NotFound("tenant", "")
			},
		},
	)

	c, rec := newCtx(t, "user-pending")
	if err := h.Me(c); err != nil {
		t.Fatalf("Me: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", rec.Code, rec.Body.String())
	}
	resp := decodeMe(t, rec)
	if resp.UserID != "user-pending" {
		t.Errorf("user_id: want user-pending, got %q", resp.UserID)
	}
	if resp.Status != "pending" {
		t.Errorf("status: want pending, got %q", resp.Status)
	}
	if resp.TenantID != nil {
		t.Errorf("tenant_id: want nil, got %v", *resp.TenantID)
	}
	if tenantCalls != 0 {
		t.Errorf("tenant repo should not be consulted for pending users; got %d calls", tenantCalls)
	}
}

// Approved user with a tenant row → 200 with tenant_id populated.
func TestMe_ApprovedUser_200_PopulatedTenant(t *testing.T) {
	approved := user.ReconstructFromData(user.UserData{
		ID:            "user-approved",
		Email:         "bob@example.com",
		OAuthProvider: "github",
		OAuthID:       "github-sub-2",
		Role:          string(user.RoleAdmin),
		Status:        string(user.StatusApproved),
		CreatedAt:     time.Now().UTC().Truncate(time.Second),
		UpdatedAt:     time.Now().UTC().Truncate(time.Second),
	})
	approvedTenant := tenant.ReconstructFromData(tenant.TenantData{
		ID:        "11111111-1111-1111-1111-111111111111",
		UserID:    "user-approved",
		DBName:    "tenant_11111111111111111111111111111111",
		Status:    string(tenant.StatusApproved),
		CreatedAt: time.Now().UTC().Truncate(time.Second),
		UpdatedAt: time.Now().UTC().Truncate(time.Second),
	})

	h := NewHandler(
		&stubUserRepo{
			getByIDFn: func(_ context.Context, _ string) (*user.User, error) {
				return approved, nil
			},
		},
		&stubTenantRepo{
			getByUserIDFn: func(_ context.Context, _ string) (*tenant.Tenant, error) {
				return approvedTenant, nil
			},
		},
	)

	c, rec := newCtx(t, "user-approved")
	if err := h.Me(c); err != nil {
		t.Fatalf("Me: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", rec.Code, rec.Body.String())
	}
	resp := decodeMe(t, rec)
	if resp.Status != "approved" {
		t.Errorf("status: want approved, got %q", resp.Status)
	}
	if resp.Role != "admin" {
		t.Errorf("role: want admin, got %q", resp.Role)
	}
	if resp.TenantID == nil {
		t.Fatal("tenant_id: want populated, got nil")
	}
	if *resp.TenantID != approvedTenant.ID() {
		t.Errorf("tenant_id: want %s, got %s", approvedTenant.ID(), *resp.TenantID)
	}
}

// Approved user but the tenant lookup blew up — fall back to nil
// tenant_id rather than 500. Documented behaviour: the dashboard's
// logout flow shouldn't be blocked on a transient platform-DB error.
func TestMe_ApprovedUser_TenantLookupError_NullTenant(t *testing.T) {
	approved := user.ReconstructFromData(user.UserData{
		ID:            "user-approved",
		Email:         "bob@example.com",
		OAuthProvider: "github",
		OAuthID:       "github-sub-2",
		Role:          string(user.RoleUser),
		Status:        string(user.StatusApproved),
		CreatedAt:     time.Now().UTC().Truncate(time.Second),
		UpdatedAt:     time.Now().UTC().Truncate(time.Second),
	})

	h := NewHandler(
		&stubUserRepo{
			getByIDFn: func(_ context.Context, _ string) (*user.User, error) {
				return approved, nil
			},
		},
		&stubTenantRepo{
			getByUserIDFn: func(_ context.Context, _ string) (*tenant.Tenant, error) {
				return nil, errors.New("platform DB unavailable")
			},
		},
	)

	c, rec := newCtx(t, "user-approved")
	if err := h.Me(c); err != nil {
		t.Fatalf("Me: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", rec.Code, rec.Body.String())
	}
	resp := decodeMe(t, rec)
	if resp.TenantID != nil {
		t.Errorf("tenant_id: want nil on tenant-lookup error, got %v", *resp.TenantID)
	}
}

// Suspended user → 200 with tenant_id=null. Dashboard renders a
// suspended-state page; tenant repo is not consulted.
func TestMe_SuspendedUser_200_NullTenant(t *testing.T) {
	suspended := user.ReconstructFromData(user.UserData{
		ID:            "user-suspended",
		Email:         "carol@example.com",
		OAuthProvider: "google",
		OAuthID:       "google-sub-3",
		Role:          string(user.RoleUser),
		Status:        string(user.StatusSuspended),
		CreatedAt:     time.Now().UTC().Truncate(time.Second),
		UpdatedAt:     time.Now().UTC().Truncate(time.Second),
	})

	tenantCalls := 0
	h := NewHandler(
		&stubUserRepo{
			getByIDFn: func(_ context.Context, _ string) (*user.User, error) {
				return suspended, nil
			},
		},
		&stubTenantRepo{
			getByUserIDFn: func(_ context.Context, _ string) (*tenant.Tenant, error) {
				tenantCalls++
				return nil, nil
			},
		},
	)

	c, rec := newCtx(t, "user-suspended")
	if err := h.Me(c); err != nil {
		t.Fatalf("Me: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", rec.Code, rec.Body.String())
	}
	resp := decodeMe(t, rec)
	if resp.Status != "suspended" {
		t.Errorf("status: want suspended, got %q", resp.Status)
	}
	if resp.TenantID != nil {
		t.Errorf("tenant_id: want nil, got %v", *resp.TenantID)
	}
	if tenantCalls != 0 {
		t.Errorf("tenant repo should not be consulted for suspended users; got %d calls", tenantCalls)
	}
}

// Constructor panics on nil deps — fail-fast convention shared with
// admin.NewHandler. Catching this at startup beats deferred 500s.
func TestNewHandler_PanicsOnNil(t *testing.T) {
	cases := []struct {
		name string
		fn   func()
	}{
		{"nil userRepo", func() { NewHandler(nil, &stubTenantRepo{}) }},
		{"nil tenantRepo", func() { NewHandler(&stubUserRepo{}, nil) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("expected panic, got none")
				}
			}()
			tc.fn()
		})
	}
}
