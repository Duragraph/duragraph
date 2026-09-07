// Hand-written admin endpoints (the operator surface at /api/admin/*). Routes
// generated into admin_gen.go; bodies live here. Targets the PLATFORM database
// (endpoints.yaml: group admin, db: platform) — users/tenants, not the
// per-tenant workflow schema.
//
// Every mutating handler here is a STATE MACHINE TRANSITION, not a patch. The
// spec gives each one a required starting state ("SELECT users WHERE id=:id AND
// status='pending'"), and that guard is load-bearing: approving an already
// approved user would re-fire tenant.provisioning and make the provisioner
// rebuild a live tenant's database. So a wrong-state transition is refused with
// 409 rather than silently doing nothing (which would look like success) or
// applying anyway (which would corrupt).
package endpoints

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"

	"github.com/duragraph/duragraph/controlplane/eventstore"
)

// adminUser is a users row as the admin surface reports it. oauth_id and the
// provider are deliberately omitted: they are credentials-adjacent identifiers
// and the operator console has no use for them.
type adminUser struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	Status    string    `json:"status"`
	TenantID  *string   `json:"tenant_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AdminListUsersResponse pairs the page with the total under the SAME filter,
// so a console can render "showing 50 of 1,284" without a second call and
// without inferring the total from a short page.
type AdminListUsersResponse struct {
	Users []adminUser `json:"users"`
	Total int         `json:"total"`
}

// maxAdminPageSize caps limit. An operator console asking for everything must
// not be able to turn one request into an unbounded scan + serialization.
const maxAdminPageSize = 200

// requireAdminIfConfigured is the auth gate for this surface.
//
// FAIL-OPEN WHEN UNCONFIGURED, AND THAT IS A REAL RISK. With no JWTSecret set,
// s.requireAdmin cannot verify anything, so rather than 500 on every call this
// lets the request through. That is tolerable ONLY because the composition root
// does not mount the platform surface without a secret, and because it keeps
// these endpoints drivable in tests that have no auth. It is NOT defence in
// depth: anyone who mounts these routes with an empty AuthConfig publishes an
// unauthenticated user-administration API.
//
// MUST BE REVISITED — the right shape is for the server to refuse to start when
// the platform surface is enabled without a secret, at which point this helper
// collapses to a plain s.requireAdmin call.
func (s *Server) requireAdminIfConfigured(c echo.Context) error {
	if len(s.Auth.JWTSecret) == 0 {
		return nil
	}
	_, err := s.requireAdmin(c)
	return err
}

// AdminListUsers lists platform users, newest-registration-last.
// GET /api/admin/users?status=&limit=&offset= -> 200 AdminListUsersResponse.
func (s *Server) AdminListUsers(c echo.Context) error {
	ctx := c.Request().Context()
	if err := s.requireAdminIfConfigured(c); err != nil {
		return err
	}

	// An unrecognised status would silently return zero rows and read as "no
	// such users" rather than "you asked a question I don't understand".
	status := c.QueryParam("status")
	if status != "" && !validUserStatus(status) {
		return echo.NewHTTPError(http.StatusUnprocessableEntity,
			"invalid status: must be pending, approved or suspended")
	}
	limit, err := intQueryParam(c, "limit", 50)
	if err != nil {
		return err
	}
	if limit > maxAdminPageSize {
		limit = maxAdminPageSize
	}
	offset, err := intQueryParam(c, "offset", 0)
	if err != nil {
		return err
	}

	// $1 = "" means unfiltered, so one query serves both cases and the count
	// below is guaranteed to use the identical predicate. Two separately
	// written predicates would be free to drift.
	rows, err := s.Platform.Query(ctx, `
		SELECT u.id, u.email, u.role, u.status, t.id AS tenant_id, u.created_at, u.updated_at
		FROM users u
		LEFT JOIN tenants t ON t.user_id = u.id
		WHERE ($1 = '' OR u.status = $1)
		ORDER BY u.created_at, u.id
		LIMIT $2 OFFSET $3`, status, limit, offset)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer rows.Close()

	out := []adminUser{} // never nil — an empty page must marshal as [], not null
	for rows.Next() {
		var u adminUser
		if err := rows.Scan(&u.ID, &u.Email, &u.Role, &u.Status, &u.TenantID, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	var total int
	if err := s.Platform.QueryRow(ctx,
		`SELECT count(*) FROM users WHERE ($1 = '' OR status = $1)`, status).Scan(&total); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, AdminListUsersResponse{Users: out, Total: total})
}

// AdminApprove admits a pending user and starts tenant provisioning.
// POST /api/admin/users/{id}/approve -> 200 adminUser / 404 / 409.
//
// tenant.provisioning is the event the platform-provisioner consumer acts on
// (endpoints.yaml side_effect): CREATE DATABASE + migrate + NATS account. It is
// therefore emitted in the SAME transaction as the status flips — if the commit
// fails, no consumer ever sees a provisioning request for a user who is not
// approved.
func (s *Server) AdminApprove(c echo.Context) error {
	return s.adminUserTransition(c, userTransition{
		from:     "pending",
		to:       "approved",
		tenantTo: "provisioning",
		events:   []string{"user.approved", "tenant.provisioning"},
	})
}

// AdminReject refuses a pending user.
// POST /api/admin/users/{id}/reject -> 200 adminUser / 404 / 409.
//
// Rejection lands the user in 'suspended', not a dedicated 'rejected' state:
// the users CHECK admits only pending|approved|suspended (001_platform), and
// the spec's step is explicit ("UPDATE users SET status='suspended'"). The
// distinction between "rejected" and "suspended later" therefore lives in the
// event log — user.rejected vs user.suspended — not in the row. The tenant is
// left untouched: it never left 'pending', so there is nothing to unwind.
func (s *Server) AdminReject(c echo.Context) error {
	return s.adminUserTransition(c, userTransition{
		from:   "pending",
		to:     "suspended",
		events: []string{"user.rejected"},
	})
}

// AdminSuspend suspends an approved user and their tenant.
// POST /api/admin/users/{id}/suspend -> 200 adminUser / 404 / 409.
func (s *Server) AdminSuspend(c echo.Context) error {
	return s.adminUserTransition(c, userTransition{
		from:     "approved",
		to:       "suspended",
		tenantTo: "suspended",
		events:   []string{"user.suspended", "tenant.suspended"},
	})
}

// AdminResume restores a suspended user and their tenant.
// POST /api/admin/users/{id}/resume -> 200 adminUser / 404 / 409.
//
// Emits NOTHING: endpoints.yaml marks this endpoint outbox:false and notes the
// user.resumed event is "not yet in spec". So this is the one transition with
// no audit trail in the event log — a real gap, but inventing an event here
// would put a type on the wire that no consumer or schema knows about.
func (s *Server) AdminResume(c echo.Context) error {
	return s.adminUserTransition(c, userTransition{
		from:     "suspended",
		to:       "approved",
		tenantTo: "approved",
	})
}

// userTransition describes one admin state change: the required starting state,
// the target, the optional tenant target, and the events to emit.
type userTransition struct {
	from     string
	to       string
	tenantTo string   // "" = leave the tenant alone
	events   []string // empty = no events (see AdminResume)
}

// adminUserTransition runs a guarded user state change.
//
// The guard and the write are ONE statement (`UPDATE ... WHERE status = from`),
// not a SELECT then an UPDATE. Two operators clicking approve simultaneously
// would both pass a separate SELECT and both emit tenant.provisioning; here the
// second UPDATE matches no row and is refused. The follow-up SELECT exists only
// to tell 404 (no such user) from 409 (wrong state) for the error message.
func (s *Server) adminUserTransition(c echo.Context, t userTransition) error {
	ctx := c.Request().Context()
	if err := s.requireAdminIfConfigured(c); err != nil {
		return err
	}
	uid, err := pathUUID(c, "id")
	if err != nil {
		return err
	}

	var out adminUser
	var tenantID *uuid.UUID
	err = s.writeTxDeferred(ctx, s.Platform, func(tx pgx.Tx) ([]eventstore.Event, error) {
		var email, role string
		cmdErr := tx.QueryRow(ctx, `
			UPDATE users SET status = $2
			WHERE id = $1 AND status = $3
			RETURNING email, role`, uid, t.to, t.from).Scan(&email, &role)
		if errors.Is(cmdErr, pgx.ErrNoRows) {
			return nil, s.explainTransitionMiss(ctx, uid, t.from)
		}
		if cmdErr != nil {
			return nil, cmdErr
		}

		// The tenant is optional: a user rejected before provisioning may have
		// one in 'pending', and a bootstrap admin always does. Update it when
		// present, and capture its id either way so the events and the response
		// can name it.
		if t.tenantTo != "" {
			var tid uuid.UUID
			terr := tx.QueryRow(ctx,
				`UPDATE tenants SET status = $2, failure_reason = NULL WHERE user_id = $1 RETURNING id`,
				uid, t.tenantTo).Scan(&tid)
			if terr == nil {
				tenantID = &tid
			} else if !errors.Is(terr, pgx.ErrNoRows) {
				return nil, terr
			}
		} else {
			var tid uuid.UUID
			terr := tx.QueryRow(ctx, `SELECT id FROM tenants WHERE user_id = $1`, uid).Scan(&tid)
			if terr == nil {
				tenantID = &tid
			} else if !errors.Is(terr, pgx.ErrNoRows) {
				return nil, terr
			}
		}

		if err := tx.QueryRow(ctx, `
			SELECT id, email, role, status, created_at, updated_at
			FROM users WHERE id = $1`, uid).
			Scan(&out.ID, &out.Email, &out.Role, &out.Status, &out.CreatedAt, &out.UpdatedAt); err != nil {
			return nil, err
		}
		if tenantID != nil {
			s := tenantID.String()
			out.TenantID = &s
		}
		return adminEvents(t.events, uid, tenantID, email, role, t.to, t.tenantTo), nil
	})
	if err != nil {
		return httpFromWriteErr(err)
	}
	return c.JSON(http.StatusOK, out)
}

// explainTransitionMiss turns "the guarded UPDATE matched nothing" into the
// right error: 404 when there is no such user, 409 naming the actual state when
// there is. Without this split every wrong-state click would read as 404 and
// look like the user had been deleted.
func (s *Server) explainTransitionMiss(ctx context.Context, uid uuid.UUID, want string) error {
	var current string
	err := s.Platform.QueryRow(ctx, `SELECT status FROM users WHERE id = $1`, uid).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}
	if err != nil {
		return err
	}
	return echo.NewHTTPError(http.StatusConflict,
		"user is "+current+"; this action requires "+want)
}

// adminEvents builds the event set for a transition. AggregateID is the REAL
// user or tenant uuid — an earlier stub emitted these against a fresh
// uuid.New(), which produced events that could never be correlated with the
// row they described.
func adminEvents(types []string, uid uuid.UUID, tenantID *uuid.UUID, email, role, userStatus, tenantStatus string) []eventstore.Event {
	out := make([]eventstore.Event, 0, len(types))
	for _, et := range types {
		switch {
		case len(et) > 5 && et[:5] == "user.":
			out = append(out, eventstore.Event{
				AggregateType: "User",
				AggregateID:   uid,
				EventType:     et,
				Payload: mustJSON(map[string]any{
					"user_id": uid.String(),
					"email":   email,
					"role":    role,
					"status":  userStatus,
				}),
			})
		case tenantID != nil:
			out = append(out, eventstore.Event{
				AggregateType: "Tenant",
				AggregateID:   *tenantID,
				EventType:     et,
				Payload: mustJSON(map[string]any{
					"tenant_id": tenantID.String(),
					"user_id":   uid.String(),
					"status":    tenantStatus,
				}),
			})
		}
		// A tenant.* event with no tenant row is dropped rather than emitted
		// against a placeholder id: there is genuinely nothing to provision.
	}
	return out
}

// AdminRetryMigration re-queues a tenant whose provisioning failed.
// POST /api/admin/tenants/{id}/retry-migration -> 200 / 404 / 409.
//
// SPEC vs SCHEMA, resolved toward the schema. endpoints.yaml prescribes
// "SELECT tenants WHERE id=:id AND status='provisioning_failed'", but the
// tenants CHECK in 001_platform admits only
// pending|provisioning|approved|failed|suspended — 'provisioning_failed' is not
// a value the column can ever hold, so the spec's guard would match zero rows
// forever and this endpoint could never do anything. The equivalent state in
// the actual schema is 'failed' (tenants.failure_reason carries the detail),
// and that is what is used here. The spec text should be corrected to match.
func (s *Server) AdminRetryMigration(c echo.Context) error {
	ctx := c.Request().Context()
	if err := s.requireAdminIfConfigured(c); err != nil {
		return err
	}
	tid, err := pathUUID(c, "id")
	if err != nil {
		return err
	}

	var userID uuid.UUID
	err = s.writeTxDeferred(ctx, s.Platform, func(tx pgx.Tx) ([]eventstore.Event, error) {
		cmdErr := tx.QueryRow(ctx, `
			UPDATE tenants SET status = 'provisioning', failure_reason = NULL
			WHERE id = $1 AND status = 'failed'
			RETURNING user_id`, tid).Scan(&userID)
		if errors.Is(cmdErr, pgx.ErrNoRows) {
			var current string
			qerr := s.Platform.QueryRow(ctx, `SELECT status FROM tenants WHERE id = $1`, tid).Scan(&current)
			if errors.Is(qerr, pgx.ErrNoRows) {
				return nil, echo.NewHTTPError(http.StatusNotFound, "tenant not found")
			}
			if qerr != nil {
				return nil, qerr
			}
			return nil, echo.NewHTTPError(http.StatusConflict,
				"tenant is "+current+"; retry requires a failed provisioning")
		}
		if cmdErr != nil {
			return nil, cmdErr
		}
		return []eventstore.Event{{
			AggregateType: "Tenant",
			AggregateID:   tid,
			EventType:     "tenant.provisioning",
			Payload: mustJSON(map[string]any{
				"tenant_id": tid.String(),
				"user_id":   userID.String(),
				"status":    "provisioning",
				"retry":     true,
			}),
		}}, nil
	})
	if err != nil {
		return httpFromWriteErr(err)
	}
	return c.JSON(http.StatusOK, map[string]any{
		"tenant_id": tid.String(),
		"status":    "provisioning",
	})
}

// AdminMetrics and AdminMetricsTenant are declared against a Mimir PromQL
// backend (endpoints.yaml: backend: mimir) that does not exist anywhere in this
// repository — there is no Mimir client, no configuration for one, and
// /metrics on the system surface only exposes the default Go/process
// collectors.
//
// They therefore answer 501 and name the missing dependency. The alternative —
// returning a plausible-looking zeroed or synthesised body, which is what the
// previous stub's empty 200 amounted to — is worse than an honest refusal: a
// dashboard cannot tell invented numbers from real ones, and "all tenants show
// zero runs" reads as an outage rather than as an unimplemented endpoint.
func (s *Server) AdminMetrics(c echo.Context) error {
	if err := s.requireAdminIfConfigured(c); err != nil {
		return err
	}
	return echo.NewHTTPError(http.StatusNotImplemented,
		"cross-tenant metrics require a Mimir backend, which is not configured in this deployment")
}

// AdminMetricsTenant — see AdminMetrics.
func (s *Server) AdminMetricsTenant(c echo.Context) error {
	if err := s.requireAdminIfConfigured(c); err != nil {
		return err
	}
	if _, err := pathUUID(c, "tenant_id"); err != nil {
		return err
	}
	return echo.NewHTTPError(http.StatusNotImplemented,
		"per-tenant metrics require a Mimir backend, which is not configured in this deployment")
}

// validUserStatus reports whether s is one of the users CHECK values.
func validUserStatus(s string) bool {
	return s == "pending" || s == "approved" || s == "suspended"
}

// intQueryParam reads an optional integer query parameter, 422ing a malformed
// one rather than letting strconv's zero value silently become the answer.
func intQueryParam(c echo.Context, name string, def int) (int, error) {
	raw := c.QueryParam(name)
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, echo.NewHTTPError(http.StatusUnprocessableEntity,
			"invalid "+name+": must be a non-negative integer")
	}
	return n, nil
}

// httpFromWriteErr passes an echo.HTTPError raised inside a transaction through
// unchanged, and turns anything else into a 500. Without this every deliberate
// 404/409 chosen inside the projection would be flattened into a 500 on the way
// out.
func httpFromWriteErr(err error) error {
	var he *echo.HTTPError
	if errors.As(err, &he) {
		return he
	}
	return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
}
