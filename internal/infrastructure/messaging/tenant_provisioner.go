// Package messaging — tenant_provisioner.go
//
// TenantProvisioner is the NATS subscriber side of the platform admin
// orchestration loop. It listens for tenant.provisioning events emitted
// by ApproveUserHandler / RetryTenantMigrationHandler and runs the
// idempotent steps that turn a pending tenant into an approved one:
//
//  1. CREATE DATABASE (the migrator wraps this in a pg_database
//     existence check; safe to retry).
//  2. golang-migrate Up (the migrator swallows ErrNoChange; safe to
//     retry).
//  3. NATS Account creation. Today this is a stubbed interface
//     (NoopNATSAccountProvisioner) — the operator-JWT wiring is not yet
//     in place; the real implementation lands in a follow-up PR.
//
// On success: tenant.Approve(SystemActorUserID, schemaVersion). On
// failure: tenant.MarkProvisioningFailed(reason). Both terminal
// transitions persist via the tenant repository, and the resulting
// tenant.approved / tenant.provisioning_failed events ride the same
// (later) audit-log subscriber path as user lifecycle events.
//
// Idempotency:
//
// A redelivered tenant.provisioning event for a tenant whose status is
// no longer `provisioning` (because a previous delivery succeeded) is
// acked-and-logged without action. Otherwise the state machine on
// tenant.Approve would reject and we'd loop on a permanent error. See
// processEvent below for the dispatch table.
//
// Bootstrap-already-approved short-circuit: when the OAuth bootstrap
// path provisioned the tenant inline, the tenant is `approved` from
// the start. If the admin then mistakenly double-approves (or some
// other path emits tenant.provisioning for that tenant), the
// subscriber must NOT re-run migrations or call tenant.Approve again.
// Same dispatch table handles this — `approved` is the no-op branch.
//
// Ack semantics:
//
// Watermill messages are process-then-ack: we call msg.Ack() only
// after the tenant Save succeeds (success or failure path). A panic
// or unrecovered error before the ack causes redelivery; the
// idempotent steps above make this safe.
package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/ThreeDotsLabs/watermill/message"

	"github.com/duragraph/duragraph/internal/application/command"
	"github.com/duragraph/duragraph/internal/domain/tenant"
	"github.com/duragraph/duragraph/internal/infrastructure/messaging/nats"
	pkgerrors "github.com/duragraph/duragraph/internal/pkg/errors"
)

// TenantProvisioningTopic is the NATS subject the subscriber listens
// on. Mirrors command.TenantProvisioningTopic so producer/consumer
// stay in lockstep — see the constant's docstring there for the
// rationale on why we don't use the bare `tenant.provisioning` subject
// from the asyncapi spec.
const TenantProvisioningTopic = command.TenantProvisioningTopic

// TenantMigrator is the subset of the postgres.Migrator API the
// provisioner needs. Keeping it as a small interface keeps the
// subscriber unit-testable without spinning up a real Postgres.
type TenantMigrator interface {
	// ProvisionTenant performs CREATE DATABASE (idempotent) and applies
	// all tenant migrations. Returns nil on success.
	ProvisionTenant(ctx context.Context, tenantID string) error

	// MigrateTenant returns the post-migration schema version for an
	// already-existing tenant DB. Used to compute the version recorded
	// on tenant.Approve.
	MigrateTenant(ctx context.Context, tenantID string) (uint, error)
}

// NATSAccountProvisioner is the operator-JWT wiring needed to create a
// per-tenant NATS Account. Today this is a stub (NoopNATSAccountProvisioner)
// because the operator JWT mode is not yet wired into the engine — the
// real implementation will land alongside the multi-account NATS
// configuration. Defined as an interface so the follow-up PR can
// substitute without changing TenantProvisioner.
type NATSAccountProvisioner interface {
	ProvisionAccount(ctx context.Context, tenantID string) error
}

// NoopNATSAccountProvisioner satisfies NATSAccountProvisioner without
// performing any work. The default while operator-JWT wiring is
// pending.
type NoopNATSAccountProvisioner struct{}

// ProvisionAccount is a no-op.
func (NoopNATSAccountProvisioner) ProvisionAccount(ctx context.Context, tenantID string) error {
	return nil
}

// TenantProvisioner is the NATS-driven worker that completes the
// async tenant provisioning workflow. Construct with NewTenantProvisioner;
// start with Run.
type TenantProvisioner struct {
	subscriber  *nats.Subscriber
	tenantRepo  tenant.Repository
	migrator    TenantMigrator
	natsAccount NATSAccountProvisioner
	logger      *log.Logger
}

// NewTenantProvisioner constructs a TenantProvisioner.
//
// natsAccount may be nil — the constructor substitutes
// NoopNATSAccountProvisioner so callers don't have to care.
func NewTenantProvisioner(
	subscriber *nats.Subscriber,
	tenantRepo tenant.Repository,
	migrator TenantMigrator,
	natsAccount NATSAccountProvisioner,
	logger *log.Logger,
) *TenantProvisioner {
	if natsAccount == nil {
		natsAccount = NoopNATSAccountProvisioner{}
	}
	if logger == nil {
		logger = log.Default()
	}
	return &TenantProvisioner{
		subscriber:  subscriber,
		tenantRepo:  tenantRepo,
		migrator:    migrator,
		natsAccount: natsAccount,
		logger:      logger,
	}
}

// Run subscribes to TenantProvisioningTopic and processes events until
// ctx is canceled. Returns the context error on cancel; any subscribe
// failure surfaces immediately.
//
// The Watermill subscriber returns one message channel per call to
// SubscribeWithContext; we drain that channel in this goroutine, ack
// or nack each message after processing, and return when the channel
// closes (which happens when ctx is canceled and the subscriber's
// internal goroutine closes the underlying NATS subscription).
func (p *TenantProvisioner) Run(ctx context.Context) error {
	ch, err := p.subscriber.SubscribeWithContext(ctx, TenantProvisioningTopic)
	if err != nil {
		return fmt.Errorf("subscribe %s: %w", TenantProvisioningTopic, err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-ch:
			if !ok {
				// Channel closed (subscription torn down). Treat as a
				// clean shutdown.
				return ctx.Err()
			}
			p.handleMessage(ctx, msg)
		}
	}
}

// handleMessage processes one Watermill message. Always acks (no
// nack) — the dispatch table inside processEvent classifies every
// outcome (success, idempotent-noop, permanent-failure) as terminal,
// and a NATS redelivery loop on a permanent failure would block the
// stream forever. We log permanent failures; an operational follow-up
// (Wave-2 audit log + dashboard) will make them visible to the admin.
func (p *TenantProvisioner) handleMessage(ctx context.Context, msg *message.Message) {
	defer msg.Ack()
	if err := p.processEvent(ctx, msg.Payload); err != nil {
		p.logger.Printf("tenant_provisioner: process event failed: %v", err)
	}
}

// processEvent unmarshals one tenant.provisioning event and runs the
// dispatch table.
func (p *TenantProvisioner) processEvent(ctx context.Context, payload []byte) error {
	tenantID, err := extractTenantID(payload)
	if err != nil {
		return fmt.Errorf("extract tenant_id: %w", err)
	}

	t, err := p.tenantRepo.GetByID(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("load tenant %s: %w", tenantID, err)
	}

	switch t.Status() {
	case tenant.StatusProvisioning:
		return p.runProvisioning(ctx, t)
	case tenant.StatusApproved:
		// Bootstrap-already-approved short-circuit (or duplicate
		// delivery after a previous successful run). No work to do.
		p.logger.Printf("tenant_provisioner: tenant %s already approved, skipping", tenantID)
		return nil
	case tenant.StatusProvisioningFailed:
		// A previous attempt already terminated in failure. The retry
		// endpoint moves the tenant back to provisioning before
		// publishing a fresh event; if we're seeing failed here it
		// means a stale redelivery — drop it.
		p.logger.Printf("tenant_provisioner: tenant %s already failed; dropping stale event", tenantID)
		return nil
	default:
		p.logger.Printf("tenant_provisioner: tenant %s in unexpected state %s; dropping event",
			tenantID, t.Status())
		return nil
	}
}

// runProvisioning is the work-doing branch: CREATE DATABASE + migrate
// + NATS Account. On success → tenant.Approve. On failure →
// tenant.MarkProvisioningFailed.
func (p *TenantProvisioner) runProvisioning(ctx context.Context, t *tenant.Tenant) error {
	// CREATE DATABASE + migrate (each step is idempotent inside the
	// migrator: pg_database existence check; golang-migrate
	// ErrNoChange).
	if err := p.migrator.ProvisionTenant(ctx, t.ID()); err != nil {
		return p.markFailed(ctx, t, fmt.Sprintf("provision: %v", err))
	}

	// Read back the schema version. MigrateTenant after ProvisionTenant
	// is the canonical way to ask "what version am I at" — golang-migrate
	// has no separate read-only Version() helper that opens a fresh
	// connection.
	version, err := p.migrator.MigrateTenant(ctx, t.ID())
	if err != nil {
		return p.markFailed(ctx, t, fmt.Sprintf("read schema version: %v", err))
	}

	// NATS Account creation — stubbed today.
	if err := p.natsAccount.ProvisionAccount(ctx, t.ID()); err != nil {
		return p.markFailed(ctx, t, fmt.Sprintf("nats account: %v", err))
	}

	// Approve. SystemActorUserID is the conventional "no human actor"
	// sentinel — see internal/domain/tenant/system_actor.go.
	if err := t.Approve(tenant.SystemActorUserID, int(version)); err != nil {
		// State-machine guard: only reachable if some racing actor
		// already moved the tenant. Surface the inconsistency in logs;
		// the next reconciliation tick will sort it out.
		return fmt.Errorf("tenant.Approve(%s): %w", t.ID(), err)
	}
	if err := p.tenantRepo.Save(ctx, t); err != nil {
		return fmt.Errorf("save approved tenant %s: %w", t.ID(), err)
	}
	p.logger.Printf("tenant_provisioner: tenant %s approved at schema_version=%d", t.ID(), version)
	return nil
}

// markFailed transitions the tenant to provisioning_failed and saves.
// Returns the original failure reason wrapped — the caller logs it —
// even if the Save succeeded, so the admin sees something happened.
func (p *TenantProvisioner) markFailed(ctx context.Context, t *tenant.Tenant, reason string) error {
	if mfErr := t.MarkProvisioningFailed(reason); mfErr != nil {
		// State-machine guard fired; log and bail.
		return fmt.Errorf("mark_failed(%s): %w", t.ID(), mfErr)
	}
	if saveErr := p.tenantRepo.Save(ctx, t); saveErr != nil {
		return fmt.Errorf("save failed-tenant %s: %w", t.ID(), saveErr)
	}
	return pkgerrors.Internal(fmt.Sprintf("tenant %s provisioning failed: %s", t.ID(), reason), nil)
}

// extractTenantID pulls the tenant_id field out of the JSON payload.
// We accept two payload shapes for robustness:
//
//  1. The bare tenant.TenantProvisioning struct (what the command
//     handlers publish today): {"tenant_id": "...", "occurred_at": ...}.
//  2. The outbox-relay envelope (in case a future wiring switch routes
//     these through the outbox): {"aggregate_id": "...", "payload": {...}}.
//     The aggregate_id of a tenant event is the tenant_id by the
//     domain.Event contract.
//
// Falling back through both keeps us forward-compatible if the
// audit-log/outbox unification PR happens before the spec/impl
// reconciliation PR.
func extractTenantID(payload []byte) (string, error) {
	var direct struct {
		TenantID    string `json:"tenant_id"`
		AggregateID string `json:"aggregate_id"`
	}
	if err := json.Unmarshal(payload, &direct); err != nil {
		return "", fmt.Errorf("unmarshal payload: %w", err)
	}
	if direct.TenantID != "" {
		return direct.TenantID, nil
	}
	if direct.AggregateID != "" {
		return direct.AggregateID, nil
	}
	return "", fmt.Errorf("payload contains neither tenant_id nor aggregate_id")
}
