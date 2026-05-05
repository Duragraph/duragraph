// Package messaging — tenant_provisioner.go
//
// TenantProvisioner is the NATS subscriber side of the platform admin
// orchestration loop. It listens (as a JetStream durable consumer —
// see DurableName below) for tenant.provisioning events emitted by
// ApproveUserHandler / RetryTenantMigrationHandler and runs the
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
// Durability:
//
// The subscriber is a JetStream durable consumer ("tenant-provisioner")
// bound to the existing duragraph-events stream filtered on subject
// duragraph.events.tenant.provisioning. Server-side state survives
// broker restarts, so a message published while the engine is offline
// is still delivered when the engine comes back. AckExplicit means a
// process crash before ack causes JetStream to redeliver after AckWait.
//
// Ack policy inside handleMessage:
//   - success / idempotent-noop / terminal-failure (tenant marked
//     provisioning_failed and saved) → Ack. Redelivering would only
//     re-run the idempotent migrator + observe the now-failed status
//     and short-circuit; no useful retry.
//   - malformed payload (extractTenantID fails) → Term. Garbage will
//     never become valid; don't loop on it.
//   - retryable error (load tenant DB hiccup, save-failed-tenant DB
//     hiccup) → Nak. The same payload re-arrives after AckWait.
//
// Idempotency:
//
// A redelivered tenant.provisioning event for a tenant whose status is
// no longer `provisioning` (because a previous delivery succeeded or
// failed) is acked-and-logged without action. The dispatch table in
// processEvent handles this — `approved` and `provisioning_failed`
// are no-op branches.
//
// Bootstrap-already-approved short-circuit: when the OAuth bootstrap
// path provisioned the tenant inline, the tenant is `approved` from
// the start. If the admin then mistakenly double-approves (or some
// other path emits tenant.provisioning for that tenant), the
// subscriber must NOT re-run migrations or call tenant.Approve again.
package messaging

import (
	"context"
	"encoding/json"
	"errors"
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
	// ProvisionTenantWithVersion performs CREATE DATABASE (idempotent),
	// applies all tenant migrations, and returns the resulting schema
	// version in one round-trip. Used to record schema_version on
	// tenant.Approve without a second migrate.Up() pass just to read
	// the version table.
	ProvisionTenantWithVersion(ctx context.Context, tenantID string) (uint, error)
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

// DurableName is the JetStream durable consumer name used by the
// tenant provisioner. Server-side delivery state is keyed off this
// name, so changing it would orphan in-flight events.
const DurableName = "tenant-provisioner"

// JetStreamStreamName is the JetStream stream the provisioner binds
// to. The publisher's ensureStreams() declares this stream with
// subjects "duragraph.events.>", which covers TenantProvisioningTopic.
const JetStreamStreamName = "duragraph-events"

// MessageSubscriber is the minimal subscribe surface the provisioner
// needs. Both *nats.JetStreamSubscriber (production) and a test fake
// satisfy it. We accept a no-arg SubscribeWithContext because the
// JetStreamSubscriber is configured for one stream+filter at
// construction time.
type MessageSubscriber interface {
	SubscribeWithContext(ctx context.Context) (<-chan *message.Message, error)
}

// TenantProvisioner is the NATS-driven worker that completes the
// async tenant provisioning workflow. Construct with NewTenantProvisioner;
// start with Run.
type TenantProvisioner struct {
	subscriber  MessageSubscriber
	tenantRepo  tenant.Repository
	migrator    TenantMigrator
	natsAccount NATSAccountProvisioner
	logger      *log.Logger
}

// NewTenantProvisioner constructs a TenantProvisioner.
//
// natsAccount may be nil — the constructor substitutes
// NoopNATSAccountProvisioner so callers don't have to care. subscriber
// MAY be nil only in unit-test paths that drive processEvent directly.
func NewTenantProvisioner(
	subscriber MessageSubscriber,
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

// Run subscribes via the configured subscriber and processes events
// until ctx is canceled. Returns the context error on cancel; any
// subscribe failure surfaces immediately.
func (p *TenantProvisioner) Run(ctx context.Context) error {
	if p.subscriber == nil {
		return fmt.Errorf("tenant provisioner: subscriber is nil")
	}
	ch, err := p.subscriber.SubscribeWithContext(ctx)
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

// handleMessage processes one message and routes the outcome to the
// JetStream message API: success / non-retryable → Ack; malformed
// payload → Term; transient infra error → Nak. See package header
// "Ack policy" for the rationale.
//
// Classification of processEvent's return value:
//   - errMalformedPayload: the bytes can't be parsed as a tenant.provisioning
//     event. Term — redelivering garbage cannot make it parseable.
//   - errTenantNotFound: payload referenced a non-existent tenant. Term —
//     no future redelivery will conjure a row that isn't there.
//   - any other non-nil error: transient infrastructure problem
//     (DB connection blip, etc.). Nak — let JetStream redeliver.
//   - nil: success or already-terminal (markFailed already persisted).
//     Ack.
func (p *TenantProvisioner) handleMessage(ctx context.Context, msg *message.Message) {
	err := p.processEvent(ctx, msg.Payload)
	switch {
	case err == nil:
		msg.Ack()
	case errors.Is(err, errMalformedPayload), errors.Is(err, errTenantNotFound):
		nats.TermMessage(msg) // best-effort; falls back to plain ack on non-JS subscribers
		msg.Ack()
	default:
		// Transient — let JetStream redeliver after AckWait.
		msg.Nack()
	}
}

// errMalformedPayload / errTenantNotFound are sentinel errors used by
// processEvent to communicate "do not retry this message" to
// handleMessage. processEvent wraps them so callers see a useful
// message; handleMessage uses errors.Is to classify.
var (
	errMalformedPayload = errors.New("tenant_provisioner: malformed payload")
	errTenantNotFound   = errors.New("tenant_provisioner: tenant not found")
)

// processEvent unmarshals one tenant.provisioning event and runs the
// dispatch table. Returns nil on success or idempotent-noop. On
// terminal failure inside runProvisioning the failure is persisted via
// markFailed and processEvent returns nil — there is nothing to retry.
// Returns non-nil only when the failure cause is transient (DB
// connection blip on initial load) OR non-retryable (sentinel errors
// errMalformedPayload / errTenantNotFound, which handleMessage maps to
// Term).
func (p *TenantProvisioner) processEvent(ctx context.Context, payload []byte) error {
	tenantID, err := extractTenantID(payload)
	if err != nil {
		// Wrap with sentinel so handleMessage knows to Term.
		return fmt.Errorf("%w: %v", errMalformedPayload, err)
	}

	t, err := p.tenantRepo.GetByID(ctx, tenantID)
	if err != nil {
		if pkgerrors.Is(err, pkgerrors.ErrNotFound) {
			return fmt.Errorf("%w: %s", errTenantNotFound, tenantID)
		}
		// Transient — let handleMessage Nak.
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
//
// Returns nil on both happy path AND terminal-failure-already-persisted
// (markFailed succeeded). Returns non-nil only when the FAILURE PATH
// itself failed to persist (state-machine guard fired, or Save failed)
// — those are transient/exceptional and benefit from a redelivery.
func (p *TenantProvisioner) runProvisioning(ctx context.Context, t *tenant.Tenant) error {
	// CREATE DATABASE + migrate + read version, in one call. Each
	// step is idempotent inside the migrator: pg_database existence
	// check; golang-migrate swallows ErrNoChange.
	version, err := p.migrator.ProvisionTenantWithVersion(ctx, t.ID())
	if err != nil {
		return p.markFailed(ctx, t, fmt.Sprintf("provision: %v", err))
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
// Returns nil when the failure has been successfully persisted —
// JetStream Acks (no useful redelivery; the next admin click goes
// through retry-migration). Returns non-nil only if the failure-path
// transition or save itself failed; those are exceptional and we
// surface so handleMessage Naks.
func (p *TenantProvisioner) markFailed(ctx context.Context, t *tenant.Tenant, reason string) error {
	if mfErr := t.MarkProvisioningFailed(reason); mfErr != nil {
		// State-machine guard fired; log and bail.
		return fmt.Errorf("mark_failed(%s): %w", t.ID(), mfErr)
	}
	if saveErr := p.tenantRepo.Save(ctx, t); saveErr != nil {
		return fmt.Errorf("save failed-tenant %s: %w", t.ID(), saveErr)
	}
	p.logger.Printf("tenant_provisioner: tenant %s marked provisioning_failed: %s", t.ID(), reason)
	return nil
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
