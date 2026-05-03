package tenant

import (
	"time"

	"github.com/duragraph/duragraph/internal/pkg/errors"
	"github.com/duragraph/duragraph/internal/pkg/eventbus"
	pkguuid "github.com/duragraph/duragraph/internal/pkg/uuid"
)

// Tenant is the aggregate root for a platform tenant.
//
// 1:1 with a User; owns a dedicated Postgres database (db_name) inside the
// shared prod-postgres instance. State machine and table-level invariants are
// defined in duragraph-spec models/entities.yml#tenants and mirrored on the
// Status type / table CHECK constraints.
type Tenant struct {
	id            string
	userID        string
	dbName        string
	status        Status
	schemaVersion *int
	provisionedAt *time.Time
	failureReason string
	createdAt     time.Time
	updatedAt     time.Time
	version       int
	events        []eventbus.Event
}

// NewTenant creates a new Tenant aggregate in pending status. The tenant ID
// is generated server-side; db_name is derived deterministically from the ID
// and stable from creation. Emits a TenantPending event.
func NewTenant(userID string) (*Tenant, error) {
	if userID == "" {
		return nil, errors.InvalidInput("user_id", "user_id is required")
	}

	tenantID := pkguuid.New()
	dbName, err := DBName(tenantID)
	if err != nil {
		// Defense-in-depth: pkguuid.New() always returns a valid UUID, so
		// this branch is unreachable in practice.
		return nil, errors.Internal("failed to derive tenant db name", err)
	}

	now := time.Now()
	t := &Tenant{
		id:        tenantID,
		userID:    userID,
		dbName:    dbName,
		status:    StatusPending,
		createdAt: now,
		updatedAt: now,
		version:   1,
		events:    make([]eventbus.Event, 0),
	}

	t.recordEvent(TenantPending{
		TenantID:   tenantID,
		UserID:     userID,
		DBName:     dbName,
		OccurredAt: now,
	})

	return t, nil
}

// StartProvisioning transitions the tenant to provisioning status.
// Valid from pending (initial provisioning) or provisioning_failed (admin
// retry). On the retry path failureReason is cleared. Emits TenantProvisioning.
func (t *Tenant) StartProvisioning() error {
	if !t.status.CanTransitionTo(StatusProvisioning) {
		return errors.InvalidState(t.status.String(), "start_provisioning")
	}

	now := time.Now()
	t.status = StatusProvisioning
	t.failureReason = ""
	t.updatedAt = now

	t.recordEvent(TenantProvisioning{
		TenantID:   t.id,
		OccurredAt: now,
	})

	return nil
}

// Approve transitions the tenant to approved status. Sets schemaVersion (the
// golang-migrate version applied during provisioning) and provisionedAt.
// Valid only from provisioning. Emits TenantApproved.
func (t *Tenant) Approve(approvedByUserID string, schemaVersion int) error {
	if !t.status.CanTransitionTo(StatusApproved) {
		return errors.InvalidState(t.status.String(), "approve")
	}

	now := time.Now()
	t.status = StatusApproved
	sv := schemaVersion
	t.schemaVersion = &sv
	t.provisionedAt = &now
	t.updatedAt = now

	t.recordEvent(TenantApproved{
		TenantID:         t.id,
		UserID:           t.userID,
		DBName:           t.dbName,
		SchemaVersion:    schemaVersion,
		ApprovedByUserID: approvedByUserID,
		OccurredAt:       now,
	})

	return nil
}

// MarkProvisioningFailed transitions the tenant to provisioning_failed status
// and records the failure reason. Valid only from provisioning. Emits
// TenantProvisioningFailed.
func (t *Tenant) MarkProvisioningFailed(reason string) error {
	if !t.status.CanTransitionTo(StatusProvisioningFailed) {
		return errors.InvalidState(t.status.String(), "mark_provisioning_failed")
	}

	now := time.Now()
	t.status = StatusProvisioningFailed
	t.failureReason = reason
	t.updatedAt = now

	t.recordEvent(TenantProvisioningFailed{
		TenantID:   t.id,
		Reason:     reason,
		OccurredAt: now,
	})

	return nil
}

// Suspend transitions the tenant to suspended status. Valid only from
// approved. An admin cannot suspend their own tenant — the self-suspend
// guard prevents an admin from accidentally locking themselves out.
//
// Order matters: state-machine validity is checked first so attempting to
// suspend a non-approved tenant surfaces the more informative InvalidState
// error rather than the self-suspend guard. Emits TenantSuspended.
func (t *Tenant) Suspend(suspendedByUserID, reason string) error {
	if !t.status.CanTransitionTo(StatusSuspended) {
		return errors.InvalidState(t.status.String(), "suspend")
	}
	if suspendedByUserID == t.userID {
		return errors.InvalidInput("suspended_by_user_id", "cannot suspend own tenant")
	}

	now := time.Now()
	t.status = StatusSuspended
	t.updatedAt = now

	t.recordEvent(TenantSuspended{
		TenantID:          t.id,
		SuspendedByUserID: suspendedByUserID,
		Reason:            reason,
		OccurredAt:        now,
	})

	return nil
}

// ID returns the tenant ID.
func (t *Tenant) ID() string { return t.id }

// UserID returns the owning user ID.
func (t *Tenant) UserID() string { return t.userID }

// DBName returns the deterministically derived Postgres database name.
func (t *Tenant) DBName() string { return t.dbName }

// Status returns the current provisioning status.
func (t *Tenant) Status() Status { return t.status }

// SchemaVersion returns the latest migrated schema version, or nil if the
// tenant has never been approved.
func (t *Tenant) SchemaVersion() *int { return t.schemaVersion }

// ProvisionedAt returns the timestamp at which the tenant first reached
// approved status, or nil if it never has.
func (t *Tenant) ProvisionedAt() *time.Time { return t.provisionedAt }

// FailureReason returns the most recent provisioning failure reason. Empty
// string in any state other than provisioning_failed.
func (t *Tenant) FailureReason() string { return t.failureReason }

// CreatedAt returns the creation timestamp.
func (t *Tenant) CreatedAt() time.Time { return t.createdAt }

// UpdatedAt returns the last-update timestamp.
func (t *Tenant) UpdatedAt() time.Time { return t.updatedAt }

// Version returns the optimistic concurrency version. Bumped by the
// repository layer on each successful Save.
func (t *Tenant) Version() int { return t.version }

// Events returns the uncommitted events recorded on this aggregate.
func (t *Tenant) Events() []eventbus.Event { return t.events }

// ClearEvents drops all uncommitted events. Called by the repository after
// a successful Save.
func (t *Tenant) ClearEvents() {
	t.events = make([]eventbus.Event, 0)
}

// recordEvent appends an event to the uncommitted events list.
func (t *Tenant) recordEvent(event eventbus.Event) {
	t.events = append(t.events, event)
}
