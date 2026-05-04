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

	// loadedUpdatedAt is the optimistic-concurrency token: the value of
	// updated_at as observed when the aggregate was loaded from the
	// projection (set by ReconstructFromData). Zero on freshly created
	// tenants — the persistence layer uses IsZero() to discriminate
	// INSERT vs UPDATE on Save. The platform.tenants table has no
	// `version` column; updated_at (maintained by the BEFORE UPDATE
	// trigger) is the OCC token.
	loadedUpdatedAt time.Time

	events []eventbus.Event
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

// Version returns the in-memory aggregate version. The platform.tenants
// projection table does not carry a version column, so this is a soft
// counter incremented by the persistence layer for diagnostic purposes;
// optimistic concurrency is enforced via LoadedUpdatedAt instead.
func (t *Tenant) Version() int { return t.version }

// LoadedUpdatedAt returns the value of updated_at observed when the
// tenant was loaded from the projection. Returns the zero time for fresh
// tenants produced by NewTenant. The persistence layer uses this as the
// optimistic-concurrency token (`WHERE id = $X AND updated_at = $Y`).
func (t *Tenant) LoadedUpdatedAt() time.Time { return t.loadedUpdatedAt }

// Events returns the uncommitted events recorded on this aggregate.
func (t *Tenant) Events() []eventbus.Event { return t.events }

// ClearEvents drops all uncommitted events. Called by the repository after
// a successful Save.
func (t *Tenant) ClearEvents() {
	t.events = make([]eventbus.Event, 0)
}

// SetPersistedState is invoked by the persistence layer immediately after
// a successful Save to refresh the OCC token (loadedUpdatedAt) and the
// authoritative updated_at observed in PG (after the BEFORE UPDATE
// trigger / column DEFAULT NOW() applied). Increments the in-memory
// version counter as a soft diagnostic.
//
// Exported so the postgres package can call it without sharing a package
// boundary; not part of the domain API used by application command
// handlers.
func (t *Tenant) SetPersistedState(updatedAt time.Time) {
	t.updatedAt = updatedAt
	t.loadedUpdatedAt = updatedAt
	t.version++
}

// recordEvent appends an event to the uncommitted events list.
func (t *Tenant) recordEvent(event eventbus.Event) {
	t.events = append(t.events, event)
}

// TenantData is a flat DTO carrying all persisted tenant fields. Used by
// ReconstructFromData to materialize a Tenant aggregate from a database
// row without going through NewTenant (which would emit a TenantPending
// event and generate a fresh ID).
type TenantData struct {
	ID            string
	UserID        string
	DBName        string
	Status        string
	SchemaVersion *int
	ProvisionedAt *time.Time
	FailureReason string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// ReconstructFromData rebuilds a Tenant aggregate from database
// projection data. The resulting aggregate has no uncommitted events and
// carries loadedUpdatedAt set to data.UpdatedAt — the OCC token used by
// the persistence layer on subsequent Save calls. The in-memory version
// counter starts at 1.
//
// Unlike NewTenant this function does NOT validate inputs (the row has
// already been validated at insert time by the table-level CHECKs); it
// does, however, fall back to StatusPending if data.Status is
// unrecognized.
func ReconstructFromData(data TenantData) *Tenant {
	status := StatusPending
	switch Status(data.Status) {
	case StatusPending, StatusProvisioning, StatusApproved,
		StatusProvisioningFailed, StatusSuspended:
		status = Status(data.Status)
	}

	return &Tenant{
		id:              data.ID,
		userID:          data.UserID,
		dbName:          data.DBName,
		status:          status,
		schemaVersion:   data.SchemaVersion,
		provisionedAt:   data.ProvisionedAt,
		failureReason:   data.FailureReason,
		createdAt:       data.CreatedAt,
		updatedAt:       data.UpdatedAt,
		version:         1,
		loadedUpdatedAt: data.UpdatedAt,
		events:          make([]eventbus.Event, 0),
	}
}
