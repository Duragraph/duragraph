package command

import (
	"context"
	"time"

	"github.com/duragraph/duragraph/internal/domain/tenant"
	"github.com/duragraph/duragraph/internal/pkg/errors"
)

// RetryTenantMigration is the input command for
// RetryTenantMigrationHandler.
//
// Used when a tenant's provisioning failed (CREATE DATABASE,
// golang-migrate.Up, or NATS Account creation hit an error). The
// tenant aggregate state machine permits provisioning_failed →
// provisioning, which re-enters the same async workflow.
type RetryTenantMigration struct {
	TenantID        string
	RetriedByUserID string
}

// RetryTenantMigrationHandler re-publishes tenant.provisioning for an
// existing tenant. The platform-provisioner subscriber re-runs the
// idempotent steps (CREATE DATABASE IF NOT EXISTS via the migrator's
// pg_database existence check, golang-migrate Up swallowing
// ErrNoChange, NATS Account creation — once that lands).
type RetryTenantMigrationHandler struct {
	tenantRepo tenant.Repository
	publisher  EventPublisher
}

// NewRetryTenantMigrationHandler constructs a
// RetryTenantMigrationHandler.
func NewRetryTenantMigrationHandler(
	tenantRepo tenant.Repository,
	publisher EventPublisher,
) *RetryTenantMigrationHandler {
	return &RetryTenantMigrationHandler{
		tenantRepo: tenantRepo,
		publisher:  publisher,
	}
}

// Handle transitions the tenant from provisioning_failed back to
// provisioning and republishes tenant.provisioning to NATS.
//
// State machine: tenant.StartProvisioning is valid from both pending
// AND provisioning_failed (per tenant/status.go). Other source states
// surface InvalidState. Idempotency: a tenant already in provisioning
// re-publishes the trigger only.
//
// RetriedByUserID is currently unused on the wire (the tenant
// aggregate's StartProvisioning does not record an actor — only the
// terminal Approve/Suspend events do). Captured for the audit log
// projection (Wave 2) and for future enrichment of the
// tenant.provisioning payload.
func (h *RetryTenantMigrationHandler) Handle(ctx context.Context, cmd RetryTenantMigration) error {
	if cmd.TenantID == "" {
		return errors.InvalidInput("tenant_id", "tenant_id is required")
	}
	if cmd.RetriedByUserID == "" {
		return errors.InvalidInput("retried_by_user_id", "retried_by_user_id is required")
	}

	t, err := h.tenantRepo.GetByID(ctx, cmd.TenantID)
	if err != nil {
		return err
	}

	// Idempotent re-publish if already provisioning.
	if t.Status() == tenant.StatusProvisioning {
		return h.publishProvisioning(ctx, t)
	}

	if err := t.StartProvisioning(); err != nil {
		return err
	}
	if err := h.tenantRepo.Save(ctx, t); err != nil {
		return errors.Internal("failed to save tenant", err)
	}

	return h.publishProvisioning(ctx, t)
}

// publishProvisioning emits the tenant.provisioning event for this
// retry attempt. Mirrors ApproveUserHandler.publishProvisioning so the
// subscriber sees a uniform payload regardless of which handler
// originated it.
func (h *RetryTenantMigrationHandler) publishProvisioning(ctx context.Context, t *tenant.Tenant) error {
	if h.publisher == nil {
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
