package endpoints

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

// truncatePlatform resets the platform tables. The pool is shared across the
// package, so every test in this file starts from a known-empty state.
func truncatePlatform(t *testing.T, ctx context.Context) {
	t.Helper()
	if _, err := testPlatform.Exec(ctx,
		`TRUNCATE users, tenants, bootstrap_lock, events, event_streams, outbox CASCADE`); err != nil {
		t.Fatal(err)
	}
}

func newAdminServer() *echo.Echo {
	e := echo.New()
	(&Server{Platform: testPlatform}).RegisterAdmin(e.Group(""))
	return e
}

// seedPlatformUser inserts a user (and, unless tenantStatus is "", their
// tenant) and returns both ids.
func seedPlatformUser(t *testing.T, ctx context.Context, email, role, status, tenantStatus string) (string, string) {
	t.Helper()
	var uid string
	if err := testPlatform.QueryRow(ctx,
		`INSERT INTO users (oauth_provider, oauth_id, email, role, status)
		 VALUES ('google', $1, $1, $2, $3) RETURNING id`, email, role, status).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	var tid string
	if tenantStatus != "" {
		if err := testPlatform.QueryRow(ctx,
			`INSERT INTO tenants (user_id, db_name, status)
			 VALUES ($1, 'tenant_' || replace(gen_random_uuid()::text,'-',''), $2) RETURNING id`,
			uid, tenantStatus).Scan(&tid); err != nil {
			t.Fatal(err)
		}
	}
	return uid, tid
}

func doAdmin(e *echo.Echo, method, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Content-Type", "application/json")
	e.ServeHTTP(rec, req)
	return rec
}

// userStatus / tenantStatus read the rows back so assertions are against the
// database rather than the handler's own response.
func userStatus(t *testing.T, ctx context.Context, uid string) string {
	t.Helper()
	var s string
	if err := testPlatform.QueryRow(ctx, `SELECT status FROM users WHERE id=$1`, uid).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func tenantStatusOf(t *testing.T, ctx context.Context, tid string) string {
	t.Helper()
	var s string
	if err := testPlatform.QueryRow(ctx, `SELECT status FROM tenants WHERE id=$1`, tid).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

// eventTypesFor returns the event types recorded against an aggregate id. The
// aggregate id is the point: a previous stub emitted these against a fresh
// uuid.New(), so events existed but could never be tied to the row they
// described. Querying BY id is what catches a regression to that.
func eventTypesFor(t *testing.T, ctx context.Context, aggID string) []string {
	t.Helper()
	rows, err := testPlatform.Query(ctx,
		`SELECT event_type FROM events WHERE aggregate_id=$1 ORDER BY id`, aggID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

// TestAdminApproveTransition covers the transition that has a real side effect:
// tenant.provisioning is what makes the provisioner build a database.
func TestAdminApproveTransition(t *testing.T) {
	ctx := context.Background()
	truncatePlatform(t, ctx)
	e := newAdminServer()

	uid, tid := seedPlatformUser(t, ctx, "approve@test", "user", "pending", "pending")

	rec := doAdmin(e, "POST", "/api/admin/users/"+uid+"/approve")
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := userStatus(t, ctx, uid); got != "approved" {
		t.Errorf("user status: want approved, got %q", got)
	}
	// The tenant must move too — approving a user without starting their
	// tenant would leave an approved account with nothing to talk to.
	if got := tenantStatusOf(t, ctx, tid); got != "provisioning" {
		t.Errorf("tenant status: want provisioning, got %q", got)
	}

	if got := eventTypesFor(t, ctx, uid); len(got) != 1 || got[0] != "user.approved" {
		t.Errorf("events on the user aggregate: want [user.approved], got %v", got)
	}
	if got := eventTypesFor(t, ctx, tid); len(got) != 1 || got[0] != "tenant.provisioning" {
		t.Errorf("events on the tenant aggregate: want [tenant.provisioning], got %v", got)
	}
}

func TestAdminSuspendResumeRoundTrip(t *testing.T) {
	ctx := context.Background()
	truncatePlatform(t, ctx)
	e := newAdminServer()

	uid, tid := seedPlatformUser(t, ctx, "suspend@test", "user", "approved", "approved")

	if rec := doAdmin(e, "POST", "/api/admin/users/"+uid+"/suspend"); rec.Code != http.StatusOK {
		t.Fatalf("suspend: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := userStatus(t, ctx, uid); got != "suspended" {
		t.Errorf("after suspend, user: want suspended, got %q", got)
	}
	if got := tenantStatusOf(t, ctx, tid); got != "suspended" {
		t.Errorf("after suspend, tenant: want suspended, got %q", got)
	}

	if rec := doAdmin(e, "POST", "/api/admin/users/"+uid+"/resume"); rec.Code != http.StatusOK {
		t.Fatalf("resume: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := userStatus(t, ctx, uid); got != "approved" {
		t.Errorf("after resume, user: want approved, got %q", got)
	}
	if got := tenantStatusOf(t, ctx, tid); got != "approved" {
		t.Errorf("after resume, tenant: want approved, got %q", got)
	}

	// resume is outbox:false in endpoints.yaml, so it must emit NOTHING. This
	// is a real audit gap, asserted here so it stays a deliberate one.
	got := eventTypesFor(t, ctx, uid)
	for _, et := range got {
		if strings.Contains(et, "resume") {
			t.Errorf("resume must not emit an event (endpoints.yaml outbox:false), got %v", got)
		}
	}
}

func TestAdminRejectLeavesTenantAlone(t *testing.T) {
	ctx := context.Background()
	truncatePlatform(t, ctx)
	e := newAdminServer()

	uid, tid := seedPlatformUser(t, ctx, "reject@test", "user", "pending", "pending")

	if rec := doAdmin(e, "POST", "/api/admin/users/"+uid+"/reject"); rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := userStatus(t, ctx, uid); got != "suspended" {
		t.Errorf("user: want suspended, got %q", got)
	}
	// Rejection never provisioned anything, so there is nothing to unwind.
	if got := tenantStatusOf(t, ctx, tid); got != "pending" {
		t.Errorf("tenant must be untouched by a rejection: want pending, got %q", got)
	}
	if got := eventTypesFor(t, ctx, uid); len(got) != 1 || got[0] != "user.rejected" {
		t.Errorf("want [user.rejected], got %v", got)
	}
}

// TestAdminWrongStateIs409 is the guard that stops a second approve from
// re-firing tenant.provisioning at a live tenant.
func TestAdminWrongStateIs409(t *testing.T) {
	ctx := context.Background()
	truncatePlatform(t, ctx)
	e := newAdminServer()

	approved, _ := seedPlatformUser(t, ctx, "already@test", "user", "approved", "approved")
	pending, _ := seedPlatformUser(t, ctx, "pending@test", "user", "pending", "pending")

	for _, tc := range []struct{ name, path string }{
		{"approve an already-approved user", "/api/admin/users/" + approved + "/approve"},
		{"reject an already-approved user", "/api/admin/users/" + approved + "/reject"},
		{"suspend a pending user", "/api/admin/users/" + pending + "/suspend"},
		{"resume a pending user", "/api/admin/users/" + pending + "/resume"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := doAdmin(e, "POST", tc.path)
			if rec.Code != http.StatusConflict {
				t.Errorf("want 409, got %d: %s", rec.Code, rec.Body.String())
			}
			// The message must name the state actually found, otherwise an
			// operator cannot tell why the action was refused.
			if !strings.Contains(rec.Body.String(), "approved") && !strings.Contains(rec.Body.String(), "pending") {
				t.Errorf("409 should name the current state, got: %s", rec.Body.String())
			}
		})
	}

	// A refused transition must not have emitted anything.
	if got := eventTypesFor(t, ctx, approved); len(got) != 0 {
		t.Errorf("a refused transition must emit no events, got %v", got)
	}
}

func TestAdminUnknownIDIs404(t *testing.T) {
	ctx := context.Background()
	truncatePlatform(t, ctx)
	e := newAdminServer()

	missing := "11111111-1111-1111-1111-111111111111"
	for _, path := range []string{
		"/api/admin/users/" + missing + "/approve",
		"/api/admin/users/" + missing + "/suspend",
		"/api/admin/tenants/" + missing + "/retry-migration",
	} {
		if rec := doAdmin(e, "POST", path); rec.Code != http.StatusNotFound {
			t.Errorf("%s: want 404, got %d: %s", path, rec.Code, rec.Body.String())
		}
	}
	// Malformed ids are a validation error, not a miss, and must not reach
	// Postgres as a 22P02.
	rec := doAdmin(e, "POST", "/api/admin/users/not-a-uuid/approve")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("malformed id: want 422, got %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "SQLSTATE") {
		t.Errorf("response leaks the driver error: %s", rec.Body.String())
	}
}

// TestAdminRetryMigration pins the spec-vs-schema resolution: endpoints.yaml
// says the guard state is 'provisioning_failed', which the tenants CHECK cannot
// hold. 'failed' is the real one.
func TestAdminRetryMigration(t *testing.T) {
	ctx := context.Background()
	truncatePlatform(t, ctx)
	e := newAdminServer()

	_, tid := seedPlatformUser(t, ctx, "retry@test", "user", "approved", "failed")
	if _, err := testPlatform.Exec(ctx,
		`UPDATE tenants SET failure_reason='disk full' WHERE id=$1`, tid); err != nil {
		t.Fatal(err)
	}

	if rec := doAdmin(e, "POST", "/api/admin/tenants/"+tid+"/retry-migration"); rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := tenantStatusOf(t, ctx, tid); got != "provisioning" {
		t.Errorf("tenant: want provisioning, got %q", got)
	}
	// The old reason must be cleared, or the next failure is indistinguishable
	// from the last one.
	var reason *string
	if err := testPlatform.QueryRow(ctx, `SELECT failure_reason FROM tenants WHERE id=$1`, tid).Scan(&reason); err != nil {
		t.Fatal(err)
	}
	if reason != nil {
		t.Errorf("failure_reason: want NULL after a retry, got %q", *reason)
	}
	if got := eventTypesFor(t, ctx, tid); len(got) != 1 || got[0] != "tenant.provisioning" {
		t.Errorf("want [tenant.provisioning], got %v", got)
	}

	// A tenant that is not failed cannot be retried.
	if rec := doAdmin(e, "POST", "/api/admin/tenants/"+tid+"/retry-migration"); rec.Code != http.StatusConflict {
		t.Errorf("retrying a non-failed tenant: want 409, got %d", rec.Code)
	}
}

func TestAdminListUsers(t *testing.T) {
	ctx := context.Background()
	truncatePlatform(t, ctx)
	e := newAdminServer()

	seedPlatformUser(t, ctx, "a@test", "admin", "approved", "approved")
	seedPlatformUser(t, ctx, "b@test", "user", "pending", "pending")
	seedPlatformUser(t, ctx, "c@test", "user", "pending", "pending")

	var resp AdminListUsersResponse
	rec := doAdmin(e, "GET", "/api/admin/users")
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Total != 3 || len(resp.Users) != 3 {
		t.Errorf("unfiltered: want 3/3, got %d/%d", len(resp.Users), resp.Total)
	}

	// The filter must constrain BOTH the page and the total, or a console
	// renders "showing 2 of 3" for a filtered view.
	rec = doAdmin(e, "GET", "/api/admin/users?status=pending")
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Total != 2 || len(resp.Users) != 2 {
		t.Errorf("status=pending: want 2/2, got %d/%d", len(resp.Users), resp.Total)
	}

	// Pagination: the total stays the FULL count while the page shrinks.
	rec = doAdmin(e, "GET", "/api/admin/users?limit=1&offset=1")
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Users) != 1 || resp.Total != 3 {
		t.Errorf("limit=1&offset=1: want 1 user of total 3, got %d/%d", len(resp.Users), resp.Total)
	}

	// An unrecognised status must not silently read as "no such users".
	if rec := doAdmin(e, "GET", "/api/admin/users?status=bogus"); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status=bogus: want 422, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec := doAdmin(e, "GET", "/api/admin/users?limit=abc"); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("limit=abc: want 422, got %d", rec.Code)
	}
}

// TestAdminMetricsAreHonestlyUnimplemented — the endpoints declare a Mimir
// backend that does not exist here. 501 is the honest answer; the previous
// stub's empty 200 was indistinguishable from "every tenant has zero activity".
func TestAdminMetricsAreHonestlyUnimplemented(t *testing.T) {
	ctx := context.Background()
	truncatePlatform(t, ctx)
	e := newAdminServer()

	for _, path := range []string{
		"/api/admin/metrics",
		"/api/admin/metrics/11111111-1111-1111-1111-111111111111",
	} {
		rec := doAdmin(e, "GET", path)
		if rec.Code != http.StatusNotImplemented {
			t.Errorf("%s: want 501, got %d: %s", path, rec.Code, rec.Body.String())
		}
		if !strings.Contains(strings.ToLower(rec.Body.String()), "mimir") {
			t.Errorf("%s: the 501 should name the missing backend, got: %s", path, rec.Body.String())
		}
	}
}
