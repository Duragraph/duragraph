// Tests for the admin HTTP handlers. The middleware chain is exercised
// in middleware/admin_auth_test.go and middleware/tenant_test.go — here
// we test handler behaviour assuming the chain is already in place
// (i.e. we seed `platform.user_id` / `platform.role` directly on the
// echo.Context so handlers see the same shape they'd get in
// production).
package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/duragraph/duragraph/internal/application/command"
	"github.com/duragraph/duragraph/internal/domain/tenant"
	"github.com/duragraph/duragraph/internal/domain/user"
	"github.com/duragraph/duragraph/internal/mocks"
)

// Context-key strings exposed for tests that need to seed identity
// claims without going through the (unexported) middleware.withXxx
// helpers. They mirror the constants in
// internal/infrastructure/http/middleware/ctxkeys.go and MUST stay in
// sync — a divergence would silently route tests around the
// middleware contract while the handler kept reading the production
// keys.
const (
	ctxKeyPlatformUserID = "platform.user_id"
	ctxKeyPlatformRole   = "platform.role"
)

// ----------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------

// fakeMetricsBackend captures issued PromQL strings and returns canned
// samples. Each entry maps a substring of the PromQL expression to the
// samples that should come back. Order-of-iteration safe: the matcher
// finds the first key whose substring appears in the query.
type fakeMetricsBackend struct {
	responses map[string][]Sample
	queries   []string
	err       error
}

func (f *fakeMetricsBackend) Query(ctx context.Context, q string) ([]Sample, error) {
	f.queries = append(f.queries, q)
	if f.err != nil {
		return nil, f.err
	}
	for needle, samples := range f.responses {
		if strings.Contains(q, needle) {
			return samples, nil
		}
	}
	return nil, nil
}

// newAdminHandler bundles construction with default mocks. Returns the
// handler plus the underlying mocks so individual tests can mutate
// repo state / inspect publish counts.
type stubs struct {
	users   *mocks.UserRepository
	tenants *mocks.TenantRepository
	pub     *mocks.EventPublisher
	metrics *fakeMetricsBackend
	handler *Handler
}

func newAdminHandler(t *testing.T) *stubs {
	t.Helper()
	users := mocks.NewUserRepository()
	tenants := mocks.NewTenantRepository()
	pub := mocks.NewEventPublisher()
	metrics := &fakeMetricsBackend{responses: map[string][]Sample{}}

	h := NewHandler(
		users,
		tenants,
		command.NewApproveUserHandler(users, tenants, pub),
		command.NewRejectUserHandler(users),
		command.NewSuspendUserHandler(users, tenants),
		command.NewResumeUserHandler(users),
		command.NewRetryTenantMigrationHandler(tenants, pub),
		metrics,
	)
	return &stubs{
		users:   users,
		tenants: tenants,
		pub:     pub,
		metrics: metrics,
		handler: h,
	}
}

// seedAdmin registers an approved admin user (bootstrap path) and
// returns it. Used as the actor on action endpoints.
func seedAdmin(t *testing.T, users *mocks.UserRepository) *user.User {
	t.Helper()
	a, err := user.RegisterUser("admin@example.com", "google", "google-admin", true)
	if err != nil {
		t.Fatalf("seedAdmin: %v", err)
	}
	if err := users.Save(context.Background(), a); err != nil {
		t.Fatalf("seedAdmin Save: %v", err)
	}
	return a
}

// seedPending registers a pending user and returns it.
func seedPending(t *testing.T, users *mocks.UserRepository, n int) *user.User {
	t.Helper()
	u, err := user.RegisterUser(
		fmt.Sprintf("user%d@example.com", n),
		"google",
		fmt.Sprintf("google-id-%d", n),
		false,
	)
	if err != nil {
		t.Fatalf("seedPending: %v", err)
	}
	if err := users.Save(context.Background(), u); err != nil {
		t.Fatalf("seedPending Save: %v", err)
	}
	return u
}

// newCtx builds an Echo context populated with the same identity keys
// the production TenantMiddleware would set. Pass adminID="" to
// simulate the missing-auth-context path (defense-in-depth 401 test).
func newCtx(t *testing.T, e *echo.Echo, method, target, body string, adminID string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, target, nil)
	}
	w := httptest.NewRecorder()
	c := e.NewContext(r, w)
	if adminID != "" {
		c.Set(ctxKeyPlatformUserID, adminID)
		c.Set(ctxKeyPlatformRole, "admin")
	}
	return c, w
}

// ----------------------------------------------------------------------
// ListUsers
// ----------------------------------------------------------------------

func TestListUsers_Success(t *testing.T) {
	s := newAdminHandler(t)
	admin := seedAdmin(t, s.users)
	seedPending(t, s.users, 1)
	seedPending(t, s.users, 2)
	seedPending(t, s.users, 3)

	e := echo.New()
	c, w := newCtx(t, e, http.MethodGet, "/api/admin/users", "", admin.ID())
	c.SetPath("/api/admin/users")
	if err := s.handler.ListUsers(c); err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}

	var resp AdminUserListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// 4 total: 1 admin + 3 pending.
	if resp.Total != 4 {
		t.Errorf("total=%d want 4", resp.Total)
	}
	if len(resp.Users) != 4 {
		t.Errorf("users=%d want 4", len(resp.Users))
	}
	if resp.Limit != defaultLimit {
		t.Errorf("limit=%d want %d", resp.Limit, defaultLimit)
	}
}

func TestListUsers_FilterByStatus(t *testing.T) {
	s := newAdminHandler(t)
	admin := seedAdmin(t, s.users)
	seedPending(t, s.users, 1)
	seedPending(t, s.users, 2)

	e := echo.New()
	c, w := newCtx(t, e, http.MethodGet, "/api/admin/users?status=pending", "", admin.ID())
	if err := s.handler.ListUsers(c); err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	var resp AdminUserListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 2 {
		t.Errorf("total=%d want 2", resp.Total)
	}
	if len(resp.Users) != 2 {
		t.Errorf("users=%d want 2", len(resp.Users))
	}
	for _, u := range resp.Users {
		if u.Status != "pending" {
			t.Errorf("expected pending, got %s", u.Status)
		}
		if u.TenantID != nil {
			t.Errorf("pending user must have nil TenantID, got %v", *u.TenantID)
		}
	}
	// Lock down the JSON-on-the-wire shape: spec says tenant_id is
	// nullable, which means it must be present with value `null`,
	// not omitted. A future `omitempty` drift would silently regress
	// this — so assert against the raw response body.
	if !strings.Contains(w.Body.String(), `"tenant_id":null`) {
		t.Errorf("expected tenant_id:null in JSON, got: %s", w.Body.String())
	}
}

func TestListUsers_Pagination(t *testing.T) {
	s := newAdminHandler(t)
	admin := seedAdmin(t, s.users)
	for i := 1; i <= 5; i++ {
		// Stagger CreatedAt so the deterministic sort gives predictable order.
		_ = seedPending(t, s.users, i)
		time.Sleep(time.Microsecond)
	}

	e := echo.New()
	c, w := newCtx(t, e, http.MethodGet, "/api/admin/users?status=pending&limit=2&offset=2", "", admin.ID())
	if err := s.handler.ListUsers(c); err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	var resp AdminUserListResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total != 5 {
		t.Errorf("total=%d want 5", resp.Total)
	}
	if len(resp.Users) != 2 {
		t.Errorf("page size=%d want 2", len(resp.Users))
	}
	if resp.Limit != 2 {
		t.Errorf("limit=%d want 2", resp.Limit)
	}
	if resp.Offset != 2 {
		t.Errorf("offset=%d want 2", resp.Offset)
	}
}

func TestListUsers_InvalidStatus(t *testing.T) {
	s := newAdminHandler(t)
	admin := seedAdmin(t, s.users)
	e := echo.New()
	c, w := newCtx(t, e, http.MethodGet, "/api/admin/users?status=banished", "", admin.ID())
	if err := s.handler.ListUsers(c); err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d want 400", w.Code)
	}
}

func TestListUsers_MissingAuth(t *testing.T) {
	s := newAdminHandler(t)
	e := echo.New()
	c, w := newCtx(t, e, http.MethodGet, "/api/admin/users", "", "" /* no admin */)
	if err := s.handler.ListUsers(c); err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status=%d want 401", w.Code)
	}
}

// ----------------------------------------------------------------------
// ApproveUser
// ----------------------------------------------------------------------

func TestApproveUser_Success(t *testing.T) {
	s := newAdminHandler(t)
	admin := seedAdmin(t, s.users)
	pending := seedPending(t, s.users, 1)

	e := echo.New()
	c, w := newCtx(t, e, http.MethodPost,
		"/api/admin/users/"+pending.ID()+"/approve", "", admin.ID())
	c.SetParamNames("user_id")
	c.SetParamValues(pending.ID())

	if err := s.handler.ApproveUser(c); err != nil {
		t.Fatalf("ApproveUser: %v", err)
	}
	if w.Code != http.StatusNoContent {
		t.Errorf("status=%d body=%s", w.Code, w.Body.String())
	}
	if got := s.users.Users[pending.ID()].Status(); got != user.StatusApproved {
		t.Errorf("status=%s want approved", got)
	}
	if s.pub.Count() != 1 {
		t.Errorf("publish count=%d want 1", s.pub.Count())
	}
}

func TestApproveUser_NotFound(t *testing.T) {
	s := newAdminHandler(t)
	admin := seedAdmin(t, s.users)
	e := echo.New()
	c, w := newCtx(t, e, http.MethodPost, "/api/admin/users/missing/approve", "", admin.ID())
	c.SetParamNames("user_id")
	c.SetParamValues("missing")
	if err := s.handler.ApproveUser(c); err != nil {
		t.Fatalf("ApproveUser: %v", err)
	}
	if w.Code != http.StatusNotFound {
		t.Errorf("status=%d want 404", w.Code)
	}
}

func TestApproveUser_InvalidState(t *testing.T) {
	// A user that has been rejected (status=suspended) cannot
	// transition back through Approve — user.Approve enforces the
	// pending → approved guard. The handler should surface 400.
	s := newAdminHandler(t)
	admin := seedAdmin(t, s.users)
	pending := seedPending(t, s.users, 1)
	if err := s.handler.reject.Handle(context.Background(), command.RejectUser{
		UserID: pending.ID(), RejectedByUserID: admin.ID(),
	}); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	c, w := newCtx(t, e, http.MethodPost, "/api/admin/users/"+pending.ID()+"/approve", "", admin.ID())
	c.SetParamNames("user_id")
	c.SetParamValues(pending.ID())
	if err := s.handler.ApproveUser(c); err != nil {
		t.Fatalf("ApproveUser: %v", err)
	}
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d want 400", w.Code)
	}
}

func TestApproveUser_MissingAuth(t *testing.T) {
	s := newAdminHandler(t)
	pending := seedPending(t, s.users, 1)
	e := echo.New()
	c, w := newCtx(t, e, http.MethodPost, "/api/admin/users/"+pending.ID()+"/approve", "", "")
	c.SetParamNames("user_id")
	c.SetParamValues(pending.ID())
	if err := s.handler.ApproveUser(c); err != nil {
		t.Fatalf("ApproveUser: %v", err)
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status=%d want 401", w.Code)
	}
}

// ----------------------------------------------------------------------
// RejectUser
// ----------------------------------------------------------------------

func TestRejectUser_Success_WithBody(t *testing.T) {
	s := newAdminHandler(t)
	admin := seedAdmin(t, s.users)
	pending := seedPending(t, s.users, 1)

	e := echo.New()
	body := `{"reason":"spam signups"}`
	c, w := newCtx(t, e, http.MethodPost, "/api/admin/users/"+pending.ID()+"/reject", body, admin.ID())
	c.SetParamNames("user_id")
	c.SetParamValues(pending.ID())
	if err := s.handler.RejectUser(c); err != nil {
		t.Fatalf("RejectUser: %v", err)
	}
	if w.Code != http.StatusNoContent {
		t.Errorf("status=%d body=%s", w.Code, w.Body.String())
	}
	if got := s.users.Users[pending.ID()].Status(); got != user.StatusSuspended {
		t.Errorf("status=%s want suspended", got)
	}
}

func TestRejectUser_Success_EmptyBody(t *testing.T) {
	// AdminActionRequest is optional per spec — empty body must work.
	s := newAdminHandler(t)
	admin := seedAdmin(t, s.users)
	pending := seedPending(t, s.users, 1)

	e := echo.New()
	c, w := newCtx(t, e, http.MethodPost, "/api/admin/users/"+pending.ID()+"/reject", "", admin.ID())
	c.SetParamNames("user_id")
	c.SetParamValues(pending.ID())
	if err := s.handler.RejectUser(c); err != nil {
		t.Fatalf("RejectUser: %v", err)
	}
	if w.Code != http.StatusNoContent {
		t.Errorf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestRejectUser_NotPending(t *testing.T) {
	s := newAdminHandler(t)
	admin := seedAdmin(t, s.users)
	// admin is approved, so rejecting them is InvalidState (state
	// machine on user.Reject permits only pending → suspended).
	e := echo.New()
	other, err := user.RegisterUser("other@example.com", "google", "other", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.users.Save(context.Background(), other); err != nil {
		t.Fatal(err)
	}
	if err := other.Approve(admin.ID()); err != nil {
		t.Fatal(err)
	}
	if err := s.users.Save(context.Background(), other); err != nil {
		t.Fatal(err)
	}

	c, w := newCtx(t, e, http.MethodPost, "/api/admin/users/"+other.ID()+"/reject", "", admin.ID())
	c.SetParamNames("user_id")
	c.SetParamValues(other.ID())
	if err := s.handler.RejectUser(c); err != nil {
		t.Fatalf("RejectUser: %v", err)
	}
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d want 400", w.Code)
	}
}

// ----------------------------------------------------------------------
// SuspendUser
// ----------------------------------------------------------------------

func TestSuspendUser_Success(t *testing.T) {
	s := newAdminHandler(t)
	admin := seedAdmin(t, s.users)
	// approve a pending user first
	pending := seedPending(t, s.users, 1)
	if err := s.handler.approve.Handle(context.Background(), command.ApproveUser{
		UserID: pending.ID(), ApprovedByUserID: admin.ID(),
	}); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	c, w := newCtx(t, e, http.MethodPost, "/api/admin/users/"+pending.ID()+"/suspend", `{"reason":"misuse"}`, admin.ID())
	c.SetParamNames("user_id")
	c.SetParamValues(pending.ID())
	if err := s.handler.SuspendUser(c); err != nil {
		t.Fatalf("SuspendUser: %v", err)
	}
	if w.Code != http.StatusNoContent {
		t.Errorf("status=%d body=%s", w.Code, w.Body.String())
	}
	if got := s.users.Users[pending.ID()].Status(); got != user.StatusSuspended {
		t.Errorf("status=%s want suspended", got)
	}
}

func TestSuspendUser_AlreadySuspendedIsIdempotent(t *testing.T) {
	// SuspendUserHandler short-circuits to no-op success on already-
	// suspended users. Verify the HTTP layer surfaces 204, not 400.
	s := newAdminHandler(t)
	admin := seedAdmin(t, s.users)
	pending := seedPending(t, s.users, 1)
	// reject → status=suspended
	if err := s.handler.reject.Handle(context.Background(), command.RejectUser{
		UserID: pending.ID(), RejectedByUserID: admin.ID(),
	}); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	c, w := newCtx(t, e, http.MethodPost, "/api/admin/users/"+pending.ID()+"/suspend", "", admin.ID())
	c.SetParamNames("user_id")
	c.SetParamValues(pending.ID())
	if err := s.handler.SuspendUser(c); err != nil {
		t.Fatalf("SuspendUser: %v", err)
	}
	if w.Code != http.StatusNoContent {
		t.Errorf("status=%d want 204", w.Code)
	}
}

// ----------------------------------------------------------------------
// ResumeUser
// ----------------------------------------------------------------------

func TestResumeUser_Success(t *testing.T) {
	s := newAdminHandler(t)
	admin := seedAdmin(t, s.users)
	pending := seedPending(t, s.users, 1)
	// reject → suspended
	if err := s.handler.reject.Handle(context.Background(), command.RejectUser{
		UserID: pending.ID(), RejectedByUserID: admin.ID(),
	}); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	c, w := newCtx(t, e, http.MethodPost, "/api/admin/users/"+pending.ID()+"/resume", "", admin.ID())
	c.SetParamNames("user_id")
	c.SetParamValues(pending.ID())
	if err := s.handler.ResumeUser(c); err != nil {
		t.Fatalf("ResumeUser: %v", err)
	}
	if w.Code != http.StatusNoContent {
		t.Errorf("status=%d body=%s", w.Code, w.Body.String())
	}
	if got := s.users.Users[pending.ID()].Status(); got != user.StatusApproved {
		t.Errorf("status=%s want approved", got)
	}
}

func TestResumeUser_NotSuspended(t *testing.T) {
	s := newAdminHandler(t)
	admin := seedAdmin(t, s.users)
	pending := seedPending(t, s.users, 1)
	// pending → resume is invalid state
	e := echo.New()
	c, w := newCtx(t, e, http.MethodPost, "/api/admin/users/"+pending.ID()+"/resume", "", admin.ID())
	c.SetParamNames("user_id")
	c.SetParamValues(pending.ID())
	if err := s.handler.ResumeUser(c); err != nil {
		t.Fatalf("ResumeUser: %v", err)
	}
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d want 400", w.Code)
	}
}

// ----------------------------------------------------------------------
// RetryTenantMigration
// ----------------------------------------------------------------------

func TestRetryTenantMigration_Success(t *testing.T) {
	s := newAdminHandler(t)
	admin := seedAdmin(t, s.users)

	// Build a tenant in provisioning_failed state.
	tn, err := tenant.NewTenant("user-uuid")
	if err != nil {
		t.Fatal(err)
	}
	if err := tn.StartProvisioning(); err != nil {
		t.Fatal(err)
	}
	if err := tn.MarkProvisioningFailed("create db error"); err != nil {
		t.Fatal(err)
	}
	if err := s.tenants.Save(context.Background(), tn); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	c, w := newCtx(t, e, http.MethodPost, "/api/admin/tenants/"+tn.ID()+"/retry-migration", "", admin.ID())
	c.SetParamNames("tenant_id")
	c.SetParamValues(tn.ID())
	if err := s.handler.RetryTenantMigration(c); err != nil {
		t.Fatalf("RetryTenantMigration: %v", err)
	}
	if w.Code != http.StatusNoContent {
		t.Errorf("status=%d body=%s", w.Code, w.Body.String())
	}
	if s.pub.Count() != 1 {
		t.Errorf("publish=%d want 1", s.pub.Count())
	}
}

func TestRetryTenantMigration_NotFound(t *testing.T) {
	s := newAdminHandler(t)
	admin := seedAdmin(t, s.users)
	e := echo.New()
	c, w := newCtx(t, e, http.MethodPost, "/api/admin/tenants/missing/retry-migration", "", admin.ID())
	c.SetParamNames("tenant_id")
	c.SetParamValues("missing")
	if err := s.handler.RetryTenantMigration(c); err != nil {
		t.Fatalf("RetryTenantMigration: %v", err)
	}
	if w.Code != http.StatusNotFound {
		t.Errorf("status=%d want 404", w.Code)
	}
}

// ----------------------------------------------------------------------
// Metrics
// ----------------------------------------------------------------------

func TestGetMetrics_HappyPath(t *testing.T) {
	s := newAdminHandler(t)
	admin := seedAdmin(t, s.users)

	// Two tenants, runs_per_sec=1.5 and 0.3, runs_active=2 and 0.
	s.metrics.responses["rate(duragraph_runs_total"] = []Sample{
		{Labels: map[string]string{"tenant_id": "tenant-A"}, Value: 1.5},
		{Labels: map[string]string{"tenant_id": "tenant-B"}, Value: 0.3},
	}
	s.metrics.responses["duragraph_runs_active"] = []Sample{
		{Labels: map[string]string{"tenant_id": "tenant-A"}, Value: 2},
	}

	e := echo.New()
	c, w := newCtx(t, e, http.MethodGet, "/api/admin/metrics", "", admin.ID())
	if err := s.handler.GetMetrics(c); err != nil {
		t.Fatalf("GetMetrics: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp AdminMetricsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Window != "5m" {
		t.Errorf("window=%s want 5m", resp.Window)
	}
	if len(resp.Tenants) != 2 {
		t.Errorf("tenants=%d want 2", len(resp.Tenants))
	}
	// Totals sum across tenants. Float comparison uses a small
	// tolerance because 1.5 + 0.3 in IEEE-754 is not exactly 1.8.
	if got := resp.Totals.RunsPerSec; math.Abs(got-1.8) > 1e-9 {
		t.Errorf("totals.runs_per_sec=%f want ~1.8", got)
	}
	if got := resp.Totals.RunsActive; got != 2 {
		t.Errorf("totals.runs_active=%d want 2", got)
	}
}

func TestGetMetrics_ServiceUnavailable_NoBackend(t *testing.T) {
	s := newAdminHandler(t)
	admin := seedAdmin(t, s.users)
	// Replace the handler's backend with nil to simulate
	// MIMIR_URL="" wiring.
	s.handler.metricsBackend = nil

	e := echo.New()
	c, w := newCtx(t, e, http.MethodGet, "/api/admin/metrics", "", admin.ID())
	if err := s.handler.GetMetrics(c); err != nil {
		t.Fatalf("GetMetrics: %v", err)
	}
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status=%d want 503", w.Code)
	}
	var body ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error != "metrics_backend_not_configured" {
		t.Errorf("error=%s want metrics_backend_not_configured", body.Error)
	}
}

func TestGetMetrics_BackendError(t *testing.T) {
	s := newAdminHandler(t)
	admin := seedAdmin(t, s.users)
	s.metrics.err = errors.New("mimir down")

	e := echo.New()
	c, w := newCtx(t, e, http.MethodGet, "/api/admin/metrics", "", admin.ID())
	if err := s.handler.GetMetrics(c); err != nil {
		t.Fatalf("GetMetrics: %v", err)
	}
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status=%d want 500", w.Code)
	}
}

func TestGetMetrics_WindowOverride(t *testing.T) {
	s := newAdminHandler(t)
	admin := seedAdmin(t, s.users)

	e := echo.New()
	c, _ := newCtx(t, e, http.MethodGet, "/api/admin/metrics?window=1h", "", admin.ID())
	if err := s.handler.GetMetrics(c); err != nil {
		t.Fatalf("GetMetrics: %v", err)
	}
	// The fake backend captured five queries; at least one rate()
	// call must have used the 1h window.
	found := false
	for _, q := range s.metrics.queries {
		if strings.Contains(q, "[1h]") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("no query contained [1h]; queries=%v", s.metrics.queries)
	}
}

// ----------------------------------------------------------------------
// Per-tenant metrics drilldown
// ----------------------------------------------------------------------

func TestGetTenantMetrics_HappyPath(t *testing.T) {
	s := newAdminHandler(t)
	admin := seedAdmin(t, s.users)
	// Seed a tenant in the projection so the tenant existence check
	// passes.
	tn, err := tenant.NewTenant("user-uuid")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.tenants.Save(context.Background(), tn); err != nil {
		t.Fatal(err)
	}

	s.metrics.responses["rate(duragraph_runs_total"] = []Sample{
		{Labels: map[string]string{"tenant_id": tn.ID()}, Value: 0.42},
	}

	e := echo.New()
	c, w := newCtx(t, e, http.MethodGet, "/api/admin/metrics/"+tn.ID(), "", admin.ID())
	c.SetParamNames("tenant_id")
	c.SetParamValues(tn.ID())
	if err := s.handler.GetTenantMetrics(c); err != nil {
		t.Fatalf("GetTenantMetrics: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp TenantMetrics
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.TenantID != tn.ID() {
		t.Errorf("tenant_id=%s want %s", resp.TenantID, tn.ID())
	}
	if resp.RunsPerSec != 0.42 {
		t.Errorf("runs_per_sec=%f want 0.42", resp.RunsPerSec)
	}
	if resp.Window != "5m" {
		t.Errorf("window=%s want 5m", resp.Window)
	}
}

func TestGetTenantMetrics_TenantNotFound(t *testing.T) {
	s := newAdminHandler(t)
	admin := seedAdmin(t, s.users)
	e := echo.New()
	c, w := newCtx(t, e, http.MethodGet, "/api/admin/metrics/missing", "", admin.ID())
	c.SetParamNames("tenant_id")
	c.SetParamValues("missing")
	if err := s.handler.GetTenantMetrics(c); err != nil {
		t.Fatalf("GetTenantMetrics: %v", err)
	}
	if w.Code != http.StatusNotFound {
		t.Errorf("status=%d want 404", w.Code)
	}
}

func TestGetTenantMetrics_ServiceUnavailable_NoBackend(t *testing.T) {
	s := newAdminHandler(t)
	admin := seedAdmin(t, s.users)
	s.handler.metricsBackend = nil

	e := echo.New()
	c, w := newCtx(t, e, http.MethodGet, "/api/admin/metrics/any", "", admin.ID())
	c.SetParamNames("tenant_id")
	c.SetParamValues("any")
	if err := s.handler.GetTenantMetrics(c); err != nil {
		t.Fatalf("GetTenantMetrics: %v", err)
	}
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status=%d want 503", w.Code)
	}
}

func TestGetTenantMetrics_NoSamplesReturnsZeros(t *testing.T) {
	// Tenant exists, Mimir returns no samples (no traffic in the
	// window). Spec semantics: 200 with zero-valued row, not 404.
	s := newAdminHandler(t)
	admin := seedAdmin(t, s.users)
	tn, err := tenant.NewTenant("user-uuid")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.tenants.Save(context.Background(), tn); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	c, w := newCtx(t, e, http.MethodGet, "/api/admin/metrics/"+tn.ID(), "", admin.ID())
	c.SetParamNames("tenant_id")
	c.SetParamValues(tn.ID())
	if err := s.handler.GetTenantMetrics(c); err != nil {
		t.Fatalf("GetTenantMetrics: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Errorf("status=%d want 200", w.Code)
	}
	var resp TenantMetrics
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.TenantID != tn.ID() {
		t.Errorf("tenant_id=%s want %s", resp.TenantID, tn.ID())
	}
	if resp.RunsPerSec != 0 || resp.RunsActive != 0 {
		t.Errorf("expected zeros, got %+v", resp)
	}
}

// ----------------------------------------------------------------------
// MimirClient — minimal happy/error-path coverage with a fake server
// ----------------------------------------------------------------------

func TestMimirClient_Query_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/prometheus/api/v1/query" {
			t.Errorf("path=%s want /prometheus/api/v1/query", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"status": "success",
			"data": {
				"resultType": "vector",
				"result": [
					{"metric": {"tenant_id": "t1"}, "value": [1700000000, "1.5"]}
				]
			}
		}`))
	}))
	defer srv.Close()

	c := NewMimirClient(srv.URL, "")
	samples, err := c.Query(context.Background(), `up`)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(samples) != 1 {
		t.Fatalf("samples=%d want 1", len(samples))
	}
	if samples[0].Value != 1.5 {
		t.Errorf("value=%f want 1.5", samples[0].Value)
	}
	if samples[0].Labels["tenant_id"] != "t1" {
		t.Errorf("label tenant_id=%s want t1", samples[0].Labels["tenant_id"])
	}
}

func TestMimirClient_Query_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewMimirClient(srv.URL, "")
	_, err := c.Query(context.Background(), `up`)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestMimirClient_TenantHeaderForwarded(t *testing.T) {
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("X-Scope-OrgID")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	}))
	defer srv.Close()

	c := NewMimirClient(srv.URL, "tenant-A")
	if _, err := c.Query(context.Background(), `up`); err != nil {
		t.Fatal(err)
	}
	if seen != "tenant-A" {
		t.Errorf("X-Scope-OrgID=%q want tenant-A", seen)
	}
}
