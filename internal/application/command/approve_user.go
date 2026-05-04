// Package command — approve_user.go
//
// ApproveUserHandler is the platform admin orchestrator command for the
// `pending → approved` user transition. It is the write side of
// `POST /api/admin/users/{user_id}/approve` (see
// duragraph-spec/api/platform.yaml).
//
// The transition has TWO aggregates in play and no shared transaction
// primitive between the User and Tenant repositories (each Save is its
// own tx; the repos are pure projection writers — see the headers on
// internal/infrastructure/persistence/postgres/{user,tenant}_repository.go).
// The handler therefore picks an order and documents the failure modes:
//
//  1. Save the User aggregate first. If this fails the operation is a
//     no-op — admin sees an error, retries.
//  2. Create or load the Tenant aggregate, call StartProvisioning, save.
//     If this fails the User row is now `approved` with no associated
//     tenant in `provisioning` state. The retry-migration endpoint and
//     the idempotency short-circuit below let the operator recover by
//     re-clicking Approve (the User stays approved, the Tenant moves
//     pending → provisioning on the second pass).
//  3. Publish `tenant.provisioning` to NATS for the platform-provisioner
//     subscriber to pick up.
//
// Idempotency strategy:
//   - If user.Status == approved AND a tenant exists already in
//     `provisioning` status: the operation is a no-op success (handles
//     the double-click / retry case where the first call partially
//     succeeded or the response was lost in flight). No re-emission of
//     events: re-driving a stuck subscriber is what
//     RetryTenantMigration is for, so keeping that responsibility on
//     one endpoint avoids accidental double-publishes from impatient
//     admin clicks.
//   - If user.Status == approved AND a tenant exists in `approved` /
//     `provisioning_failed` / `suspended` / `pending`: not a re-approve
//     scenario; admin should use the retry-migration / resume / suspend
//     endpoints instead. We return an InvalidState error.
//   - If user.Status != pending and != approved (i.e. suspended): the
//     state machine on user.Approve will reject — no separate guard
//     needed.
//
// Self-approval is blocked by user.Approve(); we do not double-check
// here.
//
// Publishing path (deliberate departure from the outbox/event-store
// flow used for run events): the User and Tenant repositories are
// projection-only by design — they do not write to the outbox. The
// audit-log mirror that consumes platform events lands in a follow-up
// PR as a separate NATS subscriber. To kick off the async provisioning
// workflow today we publish directly on the NATS publisher. See the
// EventPublisher interface below.
package command

import (
	"context"
	"time"

	"github.com/duragraph/duragraph/internal/domain/tenant"
	"github.com/duragraph/duragraph/internal/domain/user"
	"github.com/duragraph/duragraph/internal/pkg/errors"
)

// EventPublisher is the minimal pub surface needed by platform command
// handlers. *nats.Publisher satisfies this interface, but using an
// interface keeps the handler unit-testable without a NATS round-trip.
type EventPublisher interface {
	Publish(ctx context.Context, topic string, payload interface{}) error
}

// TenantProvisioningTopic is the NATS subject the platform-provisioner
// subscriber listens on. The asyncapi spec specifies bare
// `tenant.provisioning` in a `PLATFORM_TENANTS` JetStream stream that
// does not yet exist in this engine's runtime; the engine's
// publisher.ensureStreams only declares `duragraph-events`,
// `duragraph-executions`, `duragraph-runs`, and `duragraph-stream`. We
// publish under `duragraph.events.tenant.provisioning` so the message
// is captured by the existing `duragraph-events` stream (subjects
// `duragraph.events.>`) and survives broker restart. A follow-up PR
// will reconcile by adding the PLATFORM_TENANTS stream and switching to
// the bare subject — at which point this constant changes in one place.
const TenantProvisioningTopic = "duragraph.events.tenant.provisioning"

// ApproveUser is the input command for ApproveUserHandler.
type ApproveUser struct {
	// UserID is the platform.users row to approve.
	UserID string

	// ApprovedByUserID is the admin acting on this approval. Must not
	// equal UserID (self-approval guard enforced by user.Approve).
	ApprovedByUserID string
}

// ApproveUserHandler approves a pending user and kicks off async tenant
// provisioning by publishing tenant.provisioning to NATS.
type ApproveUserHandler struct {
	userRepo   user.Repository
	tenantRepo tenant.Repository
	publisher  EventPublisher
}

// NewApproveUserHandler constructs an ApproveUserHandler. publisher may
// be nil only in unit tests that explicitly assert publisher==nil
// short-circuits; production wiring always passes the real publisher.
func NewApproveUserHandler(
	userRepo user.Repository,
	tenantRepo tenant.Repository,
	publisher EventPublisher,
) *ApproveUserHandler {
	return &ApproveUserHandler{
		userRepo:   userRepo,
		tenantRepo: tenantRepo,
		publisher:  publisher,
	}
}

// Handle approves the user, ensures a tenant exists, transitions the
// tenant to provisioning, and publishes tenant.provisioning to NATS.
func (h *ApproveUserHandler) Handle(ctx context.Context, cmd ApproveUser) error {
	if cmd.UserID == "" {
		return errors.InvalidInput("user_id", "user_id is required")
	}
	if cmd.ApprovedByUserID == "" {
		return errors.InvalidInput("approved_by_user_id", "approved_by_user_id is required")
	}

	u, err := h.userRepo.GetByID(ctx, cmd.UserID)
	if err != nil {
		// Propagate NotFound / Internal as-is; the HTTP layer maps these
		// to 404 / 500 respectively.
		return err
	}

	// Idempotency short-circuit: user already approved + tenant already
	// in provisioning. No-op success — re-driving a stuck subscriber is
	// the RetryTenantMigration endpoint's job; keeping Approve out of
	// that loop avoids accidental double-publishes when an admin
	// double-clicks the Approve button.
	if u.Status() == user.StatusApproved {
		existing, getErr := h.tenantRepo.GetByUserID(ctx, cmd.UserID)
		switch {
		case getErr == nil && existing.Status() == tenant.StatusProvisioning:
			return nil
		case getErr == nil:
			// User is approved but tenant is in some other state (approved,
			// failed, suspended, pending). This is not a re-approve case.
			return errors.InvalidState(string(existing.Status()), "approve")
		case errors.Is(getErr, errors.ErrNotFound):
			// Approved user with no tenant — drop through and create one.
			// Mirrors the bootstrap-was-half-done recovery branch.
		default:
			return getErr
		}
	} else {
		// Normal pending → approved path. user.Approve enforces the state
		// machine and the self-approval guard.
		if err := u.Approve(cmd.ApprovedByUserID); err != nil {
			return err
		}
		if err := h.userRepo.Save(ctx, u); err != nil {
			return errors.Internal("failed to save user", err)
		}
	}

	// Ensure a tenant exists for this user. The OAuth bootstrap path may
	// have created one inline; the normal pending path has not.
	t, err := h.tenantRepo.GetByUserID(ctx, cmd.UserID)
	if err != nil {
		if !errors.Is(err, errors.ErrNotFound) {
			return err
		}
		t, err = tenant.NewTenant(cmd.UserID)
		if err != nil {
			return errors.Internal("failed to create tenant", err)
		}
	}

	// Transition pending → provisioning (or provisioning_failed →
	// provisioning if this is the recovery branch of an approved user
	// with a previously-failed tenant — though that case should normally
	// be routed through retry-migration; we accept it here defensively).
	if t.Status() == tenant.StatusProvisioning {
		// Already moving — no-op success. Same idempotent rationale as
		// the user.Status==approved short-circuit above.
		return nil
	}
	if err := t.StartProvisioning(); err != nil {
		return err
	}
	if err := h.tenantRepo.Save(ctx, t); err != nil {
		return errors.Internal("failed to save tenant", err)
	}

	return h.publishProvisioning(ctx, t)
}

// publishProvisioning emits the tenant.provisioning event to NATS.
// Payload shape mirrors tenant.TenantProvisioning (the canonical domain
// event from internal/domain/tenant/events.go) so a future migration
// to the outbox/audit-log path is a one-line topic change.
func (h *ApproveUserHandler) publishProvisioning(ctx context.Context, t *tenant.Tenant) error {
	if h.publisher == nil {
		// Unwired test path — caller asserted no publish.
		return nil
	}
	payload := tenant.TenantProvisioning{
		TenantID:   t.ID(),
		OccurredAt: time.Now(),
	}
	if err := h.publisher.Publish(ctx, TenantProvisioningTopic, payload); err != nil {
		return errors.Internal("failed to publish tenant.provisioning", err)
	}
	return nil
}
