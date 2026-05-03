package tenant

import (
	"errors"
	"regexp"
	"testing"

	pkgerrors "github.com/duragraph/duragraph/internal/pkg/errors"
)

const testUserID = "00000000-0000-0000-0000-000000000001"
const otherAdminID = "00000000-0000-0000-0000-0000000000aa"

var dbNamePattern = regexp.MustCompile(`^tenant_[a-f0-9]{32}$`)

// ---------------------------------------------------------------------------
// Constructor
// ---------------------------------------------------------------------------

func TestNewTenant(t *testing.T) {
	tenant, err := NewTenant(testUserID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tenant.ID() == "" {
		t.Error("tenant ID should be generated")
	}
	if tenant.UserID() != testUserID {
		t.Errorf("UserID = %q, want %q", tenant.UserID(), testUserID)
	}
	if tenant.Status() != StatusPending {
		t.Errorf("Status = %q, want %q", tenant.Status(), StatusPending)
	}
	if !dbNamePattern.MatchString(tenant.DBName()) {
		t.Errorf("DBName = %q does not match %s", tenant.DBName(), dbNamePattern)
	}
	if tenant.SchemaVersion() != nil {
		t.Error("SchemaVersion should be nil for a newly created tenant")
	}
	if tenant.ProvisionedAt() != nil {
		t.Error("ProvisionedAt should be nil for a newly created tenant")
	}
	if tenant.FailureReason() != "" {
		t.Errorf("FailureReason should be empty, got %q", tenant.FailureReason())
	}
	if tenant.Version() != 1 {
		t.Errorf("Version = %d, want 1", tenant.Version())
	}
	if tenant.CreatedAt().IsZero() {
		t.Error("CreatedAt should be set")
	}
	if tenant.UpdatedAt().IsZero() {
		t.Error("UpdatedAt should be set")
	}

	events := tenant.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventType() != EventTypeTenantPending {
		t.Errorf("event type = %q, want %q", events[0].EventType(), EventTypeTenantPending)
	}
	if events[0].AggregateID() != tenant.ID() {
		t.Errorf("event AggregateID = %q, want %q", events[0].AggregateID(), tenant.ID())
	}
	if events[0].AggregateType() != AggregateTypeTenant {
		t.Errorf("event AggregateType = %q, want %q", events[0].AggregateType(), AggregateTypeTenant)
	}

	pending, ok := events[0].(TenantPending)
	if !ok {
		t.Fatalf("event is not TenantPending: %T", events[0])
	}
	if pending.TenantID != tenant.ID() {
		t.Errorf("TenantPending.TenantID = %q, want %q", pending.TenantID, tenant.ID())
	}
	if pending.UserID != testUserID {
		t.Errorf("TenantPending.UserID = %q, want %q", pending.UserID, testUserID)
	}
	if pending.DBName != tenant.DBName() {
		t.Errorf("TenantPending.DBName = %q, want %q", pending.DBName, tenant.DBName())
	}
	if pending.OccurredAt.IsZero() {
		t.Error("TenantPending.OccurredAt should be set")
	}
}

func TestNewTenant_RejectsEmptyUserID(t *testing.T) {
	_, err := NewTenant("")
	if err == nil {
		t.Fatal("expected error for empty user_id")
	}
	if !errors.Is(err, pkgerrors.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// State machine: every valid transition + every invalid one
// ---------------------------------------------------------------------------

func TestTenant_StateMachine(t *testing.T) {
	type op string
	const (
		opStartProvisioning op = "start_provisioning"
		opApprove           op = "approve"
		opMarkFailed        op = "mark_failed"
		opSuspend           op = "suspend"
	)

	// setup builders, one per starting state.
	pendingTenant := func(t *testing.T) *Tenant {
		t.Helper()
		tenant, err := NewTenant(testUserID)
		mustNoErr(t, err)
		return tenant
	}
	provisioningTenant := func(t *testing.T) *Tenant {
		t.Helper()
		tenant := pendingTenant(t)
		mustNoErr(t, tenant.StartProvisioning())
		return tenant
	}
	approvedTenant := func(t *testing.T) *Tenant {
		t.Helper()
		tenant := provisioningTenant(t)
		mustNoErr(t, tenant.Approve(otherAdminID, 4))
		return tenant
	}
	failedTenant := func(t *testing.T) *Tenant {
		t.Helper()
		tenant := provisioningTenant(t)
		mustNoErr(t, tenant.MarkProvisioningFailed("boom"))
		return tenant
	}
	suspendedTenant := func(t *testing.T) *Tenant {
		t.Helper()
		tenant := approvedTenant(t)
		mustNoErr(t, tenant.Suspend(otherAdminID, "policy"))
		return tenant
	}

	doOp := func(t *Tenant, o op) error {
		switch o {
		case opStartProvisioning:
			return t.StartProvisioning()
		case opApprove:
			return t.Approve(otherAdminID, 1)
		case opMarkFailed:
			return t.MarkProvisioningFailed("err")
		case opSuspend:
			return t.Suspend(otherAdminID, "policy")
		}
		panic("unknown op")
	}

	tests := []struct {
		name      string
		setup     func(t *testing.T) *Tenant
		op        op
		wantState Status
		wantErr   bool
	}{
		// Valid transitions
		{"pending->provisioning", pendingTenant, opStartProvisioning, StatusProvisioning, false},
		{"provisioning->approved", provisioningTenant, opApprove, StatusApproved, false},
		{"provisioning->provisioning_failed", provisioningTenant, opMarkFailed, StatusProvisioningFailed, false},
		{"provisioning_failed->provisioning (retry)", failedTenant, opStartProvisioning, StatusProvisioning, false},
		{"approved->suspended", approvedTenant, opSuspend, StatusSuspended, false},

		// Invalid: pending
		{"pending cannot approve", pendingTenant, opApprove, "", true},
		{"pending cannot mark_failed", pendingTenant, opMarkFailed, "", true},
		{"pending cannot suspend", pendingTenant, opSuspend, "", true},

		// Invalid: provisioning
		{"provisioning cannot start_provisioning", provisioningTenant, opStartProvisioning, "", true},
		{"provisioning cannot suspend", provisioningTenant, opSuspend, "", true},

		// Invalid: approved
		{"approved cannot start_provisioning", approvedTenant, opStartProvisioning, "", true},
		{"approved cannot approve", approvedTenant, opApprove, "", true},
		{"approved cannot mark_failed", approvedTenant, opMarkFailed, "", true},

		// Invalid: provisioning_failed
		{"provisioning_failed cannot approve", failedTenant, opApprove, "", true},
		{"provisioning_failed cannot mark_failed", failedTenant, opMarkFailed, "", true},
		{"provisioning_failed cannot suspend", failedTenant, opSuspend, "", true},

		// Invalid: suspended (terminal)
		{"suspended cannot start_provisioning", suspendedTenant, opStartProvisioning, "", true},
		{"suspended cannot approve", suspendedTenant, opApprove, "", true},
		{"suspended cannot mark_failed", suspendedTenant, opMarkFailed, "", true},
		{"suspended cannot suspend", suspendedTenant, opSuspend, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tenant := tt.setup(t)
			err := doOp(tenant, tt.op)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (state=%q)", tenant.Status())
				}
				if !errors.Is(err, pkgerrors.ErrInvalidState) {
					t.Errorf("expected ErrInvalidState, got %v", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tenant.Status() != tt.wantState {
				t.Errorf("Status = %q, want %q", tenant.Status(), tt.wantState)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Approve sets schema version + provisioned_at and emits the right event
// ---------------------------------------------------------------------------

func TestTenant_Approve_SetsSchemaAndTimestamp(t *testing.T) {
	tenant, err := NewTenant(testUserID)
	mustNoErr(t, err)
	mustNoErr(t, tenant.StartProvisioning())

	mustNoErr(t, tenant.Approve(otherAdminID, 7))

	if tenant.Status() != StatusApproved {
		t.Errorf("Status = %q, want %q", tenant.Status(), StatusApproved)
	}
	if tenant.SchemaVersion() == nil {
		t.Fatal("SchemaVersion should be set after Approve")
	}
	if *tenant.SchemaVersion() != 7 {
		t.Errorf("SchemaVersion = %d, want 7", *tenant.SchemaVersion())
	}
	if tenant.ProvisionedAt() == nil {
		t.Fatal("ProvisionedAt should be set after Approve")
	}

	events := tenant.Events()
	if len(events) != 3 {
		t.Fatalf("expected 3 events (pending+provisioning+approved), got %d", len(events))
	}
	approved, ok := events[2].(TenantApproved)
	if !ok {
		t.Fatalf("event[2] is not TenantApproved: %T", events[2])
	}
	if approved.SchemaVersion != 7 {
		t.Errorf("TenantApproved.SchemaVersion = %d, want 7", approved.SchemaVersion)
	}
	if approved.ApprovedByUserID != otherAdminID {
		t.Errorf("TenantApproved.ApprovedByUserID = %q, want %q", approved.ApprovedByUserID, otherAdminID)
	}
	if approved.DBName != tenant.DBName() {
		t.Errorf("TenantApproved.DBName = %q, want %q", approved.DBName, tenant.DBName())
	}
	if approved.UserID != testUserID {
		t.Errorf("TenantApproved.UserID = %q, want %q", approved.UserID, testUserID)
	}
}

// ---------------------------------------------------------------------------
// provisioning_failed -> provisioning retry path clears failure_reason
// ---------------------------------------------------------------------------

func TestTenant_RetryFromFailed(t *testing.T) {
	tenant, err := NewTenant(testUserID)
	mustNoErr(t, err)
	mustNoErr(t, tenant.StartProvisioning())
	mustNoErr(t, tenant.MarkProvisioningFailed("create database failed"))

	if tenant.Status() != StatusProvisioningFailed {
		t.Fatalf("Status = %q, want %q", tenant.Status(), StatusProvisioningFailed)
	}
	if tenant.FailureReason() != "create database failed" {
		t.Errorf("FailureReason = %q, want 'create database failed'", tenant.FailureReason())
	}

	mustNoErr(t, tenant.StartProvisioning())

	if tenant.Status() != StatusProvisioning {
		t.Errorf("Status = %q, want %q", tenant.Status(), StatusProvisioning)
	}
	if tenant.FailureReason() != "" {
		t.Errorf("FailureReason should be cleared on retry, got %q", tenant.FailureReason())
	}

	// Event log: pending, provisioning, provisioning_failed, provisioning
	events := tenant.Events()
	wantTypes := []string{
		EventTypeTenantPending,
		EventTypeTenantProvisioning,
		EventTypeTenantProvisioningFailed,
		EventTypeTenantProvisioning,
	}
	if len(events) != len(wantTypes) {
		t.Fatalf("got %d events, want %d", len(events), len(wantTypes))
	}
	for i, want := range wantTypes {
		if events[i].EventType() != want {
			t.Errorf("event[%d] = %q, want %q", i, events[i].EventType(), want)
		}
	}
}

// ---------------------------------------------------------------------------
// Self-suspend guard
// ---------------------------------------------------------------------------

func TestTenant_Suspend_SelfSuspendBlocked(t *testing.T) {
	tenant, err := NewTenant(testUserID)
	mustNoErr(t, err)
	mustNoErr(t, tenant.StartProvisioning())
	mustNoErr(t, tenant.Approve(otherAdminID, 1))

	err = tenant.Suspend(testUserID, "trying to self-suspend")
	if err == nil {
		t.Fatal("expected error when admin tries to suspend own tenant")
	}
	if !errors.Is(err, pkgerrors.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
	if tenant.Status() != StatusApproved {
		t.Errorf("status should be unchanged after blocked self-suspend, got %q", tenant.Status())
	}

	// And another admin can suspend.
	mustNoErr(t, tenant.Suspend(otherAdminID, "policy"))
	if tenant.Status() != StatusSuspended {
		t.Errorf("status = %q, want %q", tenant.Status(), StatusSuspended)
	}
}

// ---------------------------------------------------------------------------
// ClearEvents
// ---------------------------------------------------------------------------

func TestTenant_ClearEvents(t *testing.T) {
	tenant, err := NewTenant(testUserID)
	mustNoErr(t, err)
	if len(tenant.Events()) == 0 {
		t.Fatal("tenant should have events after creation")
	}
	tenant.ClearEvents()
	if len(tenant.Events()) != 0 {
		t.Errorf("Events should be empty after ClearEvents, got %d", len(tenant.Events()))
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func mustNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
