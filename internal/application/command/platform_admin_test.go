package command

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/duragraph/duragraph/internal/domain/tenant"
	"github.com/duragraph/duragraph/internal/domain/user"
	"github.com/duragraph/duragraph/internal/mocks"
	pkgerrors "github.com/duragraph/duragraph/internal/pkg/errors"
)

// helper: register a normal pending user, return the persisted *user.User.
func seedPendingUser(t *testing.T, repo *mocks.UserRepository) *user.User {
	t.Helper()
	u, err := user.RegisterUser("alice@example.com", "google", "google-id-1", false)
	if err != nil {
		t.Fatalf("seedPendingUser: RegisterUser: %v", err)
	}
	if err := repo.Save(context.Background(), u); err != nil {
		t.Fatalf("seedPendingUser: Save: %v", err)
	}
	return u
}

// helper: seed an admin user (bootstrap path) so we have a separate
// approver.
func seedAdminUser(t *testing.T, repo *mocks.UserRepository) *user.User {
	t.Helper()
	a, err := user.RegisterUser("admin@example.com", "google", "google-admin", true)
	if err != nil {
		t.Fatalf("seedAdminUser: RegisterUser: %v", err)
	}
	if err := repo.Save(context.Background(), a); err != nil {
		t.Fatalf("seedAdminUser: Save: %v", err)
	}
	return a
}

// =================
// ApproveUserHandler
// =================

func TestApproveUserHandler_Success(t *testing.T) {
	uRepo := mocks.NewUserRepository()
	tRepo := mocks.NewTenantRepository()
	pub := mocks.NewEventPublisher()
	admin := seedAdminUser(t, uRepo)
	pending := seedPendingUser(t, uRepo)

	h := NewApproveUserHandler(uRepo, tRepo, pub)
	if err := h.Handle(context.Background(), ApproveUser{
		UserID:           pending.ID(),
		ApprovedByUserID: admin.ID(),
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if uRepo.Users[pending.ID()].Status() != user.StatusApproved {
		t.Fatalf("user should be approved, got %s", uRepo.Users[pending.ID()].Status())
	}
	if len(tRepo.Tenants) != 1 {
		t.Fatalf("expected 1 tenant created, got %d", len(tRepo.Tenants))
	}
	for _, te := range tRepo.Tenants {
		if te.Status() != tenant.StatusProvisioning {
			t.Errorf("tenant should be provisioning, got %s", te.Status())
		}
		if te.UserID() != pending.ID() {
			t.Errorf("tenant.UserID mismatch: got %s want %s", te.UserID(), pending.ID())
		}
	}
	if pub.Count() != 1 {
		t.Errorf("expected 1 publish, got %d", pub.Count())
	}
	if pub.Events[0].Topic != TenantProvisioningTopic {
		t.Errorf("expected topic %s, got %s", TenantProvisioningTopic, pub.Events[0].Topic)
	}
}

func TestApproveUserHandler_UserNotFound(t *testing.T) {
	uRepo := mocks.NewUserRepository()
	tRepo := mocks.NewTenantRepository()
	pub := mocks.NewEventPublisher()
	h := NewApproveUserHandler(uRepo, tRepo, pub)
	err := h.Handle(context.Background(), ApproveUser{
		UserID:           "nonexistent",
		ApprovedByUserID: "admin-id",
	})
	if err == nil {
		t.Fatal("expected error for missing user")
	}
	if !pkgerrors.Is(err, pkgerrors.ErrNotFound) {
		t.Errorf("expected NotFound, got %v", err)
	}
}

func TestApproveUserHandler_SelfApprovalBlocked(t *testing.T) {
	uRepo := mocks.NewUserRepository()
	tRepo := mocks.NewTenantRepository()
	pub := mocks.NewEventPublisher()
	pending := seedPendingUser(t, uRepo)

	h := NewApproveUserHandler(uRepo, tRepo, pub)
	err := h.Handle(context.Background(), ApproveUser{
		UserID:           pending.ID(),
		ApprovedByUserID: pending.ID(),
	})
	if err == nil {
		t.Fatal("expected self-approval to be rejected")
	}
}

func TestApproveUserHandler_AlreadyApprovedDoubleClick(t *testing.T) {
	// Idempotency short-circuit: user already approved + tenant already
	// provisioning. Should be a no-op success WITHOUT re-publishing —
	// re-driving a stuck subscriber is the RetryTenantMigration
	// endpoint's job, not Approve's.
	uRepo := mocks.NewUserRepository()
	tRepo := mocks.NewTenantRepository()
	pub := mocks.NewEventPublisher()
	admin := seedAdminUser(t, uRepo)
	pending := seedPendingUser(t, uRepo)

	h := NewApproveUserHandler(uRepo, tRepo, pub)
	if err := h.Handle(context.Background(), ApproveUser{
		UserID:           pending.ID(),
		ApprovedByUserID: admin.ID(),
	}); err != nil {
		t.Fatalf("first call unexpected error: %v", err)
	}
	// Second call — same user, same admin. No-op success, no publish.
	if err := h.Handle(context.Background(), ApproveUser{
		UserID:           pending.ID(),
		ApprovedByUserID: admin.ID(),
	}); err != nil {
		t.Fatalf("second call should be idempotent, got: %v", err)
	}
	if len(tRepo.Tenants) != 1 {
		t.Errorf("should still have 1 tenant, got %d", len(tRepo.Tenants))
	}
	if pub.Count() != 1 {
		t.Errorf("expected 1 publish (no re-issue on idempotent path), got %d", pub.Count())
	}
}

func TestApproveUserHandler_AlreadyApprovedTenantInWrongState(t *testing.T) {
	// User is approved but tenant is in approved state — caller should
	// use other endpoints, not Approve. We surface InvalidState.
	uRepo := mocks.NewUserRepository()
	tRepo := mocks.NewTenantRepository()
	pub := mocks.NewEventPublisher()
	admin := seedAdminUser(t, uRepo)
	pending := seedPendingUser(t, uRepo)
	if err := pending.Approve(admin.ID()); err != nil {
		t.Fatalf("setup: approve user: %v", err)
	}
	if err := uRepo.Save(context.Background(), pending); err != nil {
		t.Fatalf("setup: save user: %v", err)
	}
	te, err := tenant.NewTenant(pending.ID())
	if err != nil {
		t.Fatalf("setup: NewTenant: %v", err)
	}
	if err := te.StartProvisioning(); err != nil {
		t.Fatalf("setup: StartProvisioning: %v", err)
	}
	if err := te.Approve(admin.ID(), 5); err != nil {
		t.Fatalf("setup: Approve: %v", err)
	}
	if err := tRepo.Save(context.Background(), te); err != nil {
		t.Fatalf("setup: save tenant: %v", err)
	}

	h := NewApproveUserHandler(uRepo, tRepo, pub)
	err = h.Handle(context.Background(), ApproveUser{
		UserID:           pending.ID(),
		ApprovedByUserID: admin.ID(),
	})
	if err == nil {
		t.Fatal("expected InvalidState error")
	}
	if !pkgerrors.Is(err, pkgerrors.ErrInvalidState) {
		t.Errorf("expected InvalidState, got %v", err)
	}
}

func TestApproveUserHandler_BootstrapHalfDoneRecovery(t *testing.T) {
	// Edge case: user already approved (e.g. crash mid-bootstrap), no
	// tenant exists. Approve should create the tenant and provision.
	uRepo := mocks.NewUserRepository()
	tRepo := mocks.NewTenantRepository()
	pub := mocks.NewEventPublisher()
	admin := seedAdminUser(t, uRepo)
	pending := seedPendingUser(t, uRepo)
	if err := pending.Approve(admin.ID()); err != nil {
		t.Fatalf("setup approve: %v", err)
	}
	if err := uRepo.Save(context.Background(), pending); err != nil {
		t.Fatalf("setup save: %v", err)
	}

	h := NewApproveUserHandler(uRepo, tRepo, pub)
	err := h.Handle(context.Background(), ApproveUser{
		UserID:           pending.ID(),
		ApprovedByUserID: admin.ID(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tRepo.Tenants) != 1 {
		t.Errorf("expected tenant created on recovery, got %d", len(tRepo.Tenants))
	}
}

func TestApproveUserHandler_AlreadyApprovedFailedTenantRecovers(t *testing.T) {
	// Edge case: user is approved + tenant in `provisioning_failed`
	// (a previous approval got past user.Save but the async
	// provisioning ultimately failed). Re-clicking Approve should
	// fall through to StartProvisioning + publish so the admin can
	// recover without going to the dedicated retry-migration
	// endpoint. This is the #4 review-comment fix; without it the
	// idempotency branch returns InvalidState and the admin's
	// natural "click Approve again" gesture is rejected.
	uRepo := mocks.NewUserRepository()
	tRepo := mocks.NewTenantRepository()
	pub := mocks.NewEventPublisher()
	admin := seedAdminUser(t, uRepo)
	pending := seedPendingUser(t, uRepo)
	if err := pending.Approve(admin.ID()); err != nil {
		t.Fatalf("setup Approve: %v", err)
	}
	if err := uRepo.Save(context.Background(), pending); err != nil {
		t.Fatalf("setup Save user: %v", err)
	}
	te, err := tenant.NewTenant(pending.ID())
	if err != nil {
		t.Fatalf("setup NewTenant: %v", err)
	}
	if err := te.StartProvisioning(); err != nil {
		t.Fatalf("setup StartProvisioning: %v", err)
	}
	if err := te.MarkProvisioningFailed("prior failure"); err != nil {
		t.Fatalf("setup MarkProvisioningFailed: %v", err)
	}
	if err := tRepo.Save(context.Background(), te); err != nil {
		t.Fatalf("setup Save tenant: %v", err)
	}

	h := NewApproveUserHandler(uRepo, tRepo, pub)
	if err := h.Handle(context.Background(), ApproveUser{
		UserID:           pending.ID(),
		ApprovedByUserID: admin.ID(),
	}); err != nil {
		t.Fatalf("approve should recover from provisioning_failed, got: %v", err)
	}
	if got := tRepo.Tenants[te.ID()].Status(); got != tenant.StatusProvisioning {
		t.Errorf("tenant should be back in provisioning, got %s", got)
	}
	if pub.Count() != 1 {
		t.Errorf("expected 1 publish on recovery path, got %d", pub.Count())
	}
}

func TestApproveUserHandler_NilPublisherPanics(t *testing.T) {
	// Constructor must panic when publisher is nil — a misconfigured
	// handler that silently drops the trigger appears to succeed but
	// never starts the async workflow. #5 review-comment fix.
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when constructing with nil publisher")
		}
	}()
	NewApproveUserHandler(mocks.NewUserRepository(), mocks.NewTenantRepository(), nil)
}

func TestRetryTenantMigrationHandler_NilPublisherPanics(t *testing.T) {
	// Same rationale as ApproveUserHandler — constructor enforces
	// non-nil publisher.
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when constructing with nil publisher")
		}
	}()
	NewRetryTenantMigrationHandler(mocks.NewTenantRepository(), nil)
}

func TestApproveUserHandler_SaveError(t *testing.T) {
	uRepo := mocks.NewUserRepository()
	tRepo := mocks.NewTenantRepository()
	pub := mocks.NewEventPublisher()
	admin := seedAdminUser(t, uRepo)
	pending := seedPendingUser(t, uRepo)
	uRepo.SaveFunc = func(ctx context.Context, u *user.User) error {
		return fmt.Errorf("db error")
	}

	h := NewApproveUserHandler(uRepo, tRepo, pub)
	err := h.Handle(context.Background(), ApproveUser{
		UserID:           pending.ID(),
		ApprovedByUserID: admin.ID(),
	})
	if err == nil {
		t.Fatal("expected error from save")
	}
	if pub.Count() != 0 {
		t.Errorf("should not publish on save failure, got %d publishes", pub.Count())
	}
}

// ================
// RejectUserHandler
// ================

func TestRejectUserHandler_Success(t *testing.T) {
	uRepo := mocks.NewUserRepository()
	admin := seedAdminUser(t, uRepo)
	pending := seedPendingUser(t, uRepo)

	h := NewRejectUserHandler(uRepo)
	if err := h.Handle(context.Background(), RejectUser{
		UserID:           pending.ID(),
		RejectedByUserID: admin.ID(),
		Reason:           "spam",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uRepo.Users[pending.ID()].Status() != user.StatusSuspended {
		t.Errorf("expected suspended status, got %s", uRepo.Users[pending.ID()].Status())
	}
}

func TestRejectUserHandler_NotPending(t *testing.T) {
	uRepo := mocks.NewUserRepository()
	admin := seedAdminUser(t, uRepo)
	pending := seedPendingUser(t, uRepo)
	if err := pending.Approve(admin.ID()); err != nil {
		t.Fatalf("setup approve: %v", err)
	}
	if err := uRepo.Save(context.Background(), pending); err != nil {
		t.Fatalf("setup save: %v", err)
	}

	h := NewRejectUserHandler(uRepo)
	err := h.Handle(context.Background(), RejectUser{
		UserID:           pending.ID(),
		RejectedByUserID: admin.ID(),
	})
	if err == nil {
		t.Fatal("expected InvalidState for non-pending user")
	}
}

func TestRejectUserHandler_SelfReject(t *testing.T) {
	uRepo := mocks.NewUserRepository()
	pending := seedPendingUser(t, uRepo)

	h := NewRejectUserHandler(uRepo)
	err := h.Handle(context.Background(), RejectUser{
		UserID:           pending.ID(),
		RejectedByUserID: pending.ID(),
	})
	if err == nil {
		t.Fatal("expected self-reject to be blocked")
	}
}

// =================
// SuspendUserHandler
// =================

func TestSuspendUserHandler_Success(t *testing.T) {
	uRepo := mocks.NewUserRepository()
	tRepo := mocks.NewTenantRepository()
	admin := seedAdminUser(t, uRepo)
	pending := seedPendingUser(t, uRepo)
	if err := pending.Approve(admin.ID()); err != nil {
		t.Fatalf("setup approve: %v", err)
	}
	if err := uRepo.Save(context.Background(), pending); err != nil {
		t.Fatalf("setup save: %v", err)
	}
	te, err := tenant.NewTenant(pending.ID())
	if err != nil {
		t.Fatalf("setup NewTenant: %v", err)
	}
	if err := te.StartProvisioning(); err != nil {
		t.Fatalf("setup StartProvisioning: %v", err)
	}
	if err := te.Approve(admin.ID(), 5); err != nil {
		t.Fatalf("setup Approve: %v", err)
	}
	if err := tRepo.Save(context.Background(), te); err != nil {
		t.Fatalf("setup Save: %v", err)
	}

	h := NewSuspendUserHandler(uRepo, tRepo)
	if err := h.Handle(context.Background(), SuspendUser{
		UserID:            pending.ID(),
		SuspendedByUserID: admin.ID(),
		Reason:            "violation",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uRepo.Users[pending.ID()].Status() != user.StatusSuspended {
		t.Errorf("user should be suspended")
	}
	if tRepo.Tenants[te.ID()].Status() != tenant.StatusSuspended {
		t.Errorf("tenant should be suspended, got %s", tRepo.Tenants[te.ID()].Status())
	}
}

func TestSuspendUserHandler_AlreadySuspended(t *testing.T) {
	uRepo := mocks.NewUserRepository()
	tRepo := mocks.NewTenantRepository()
	admin := seedAdminUser(t, uRepo)
	pending := seedPendingUser(t, uRepo)
	if err := pending.Reject(admin.ID(), "spam"); err != nil { // rejected → suspended
		t.Fatalf("setup Reject: %v", err)
	}
	if err := uRepo.Save(context.Background(), pending); err != nil {
		t.Fatalf("setup Save: %v", err)
	}

	h := NewSuspendUserHandler(uRepo, tRepo)
	if err := h.Handle(context.Background(), SuspendUser{
		UserID:            pending.ID(),
		SuspendedByUserID: admin.ID(),
	}); err != nil {
		t.Fatalf("idempotent suspend should succeed, got %v", err)
	}
}

func TestSuspendUserHandler_NoTenantOK(t *testing.T) {
	// Pending user with no tenant: suspend the user only (no-op on
	// tenant). Use Reject path to reach suspended status without a
	// tenant; here we skip directly via approve+suspend flow instead.
	uRepo := mocks.NewUserRepository()
	tRepo := mocks.NewTenantRepository()
	admin := seedAdminUser(t, uRepo)
	pending := seedPendingUser(t, uRepo)
	if err := pending.Approve(admin.ID()); err != nil {
		t.Fatalf("setup approve: %v", err)
	}
	if err := uRepo.Save(context.Background(), pending); err != nil {
		t.Fatalf("setup save: %v", err)
	}

	h := NewSuspendUserHandler(uRepo, tRepo)
	if err := h.Handle(context.Background(), SuspendUser{
		UserID:            pending.ID(),
		SuspendedByUserID: admin.ID(),
	}); err != nil {
		t.Fatalf("suspend with no tenant should succeed, got %v", err)
	}
	if uRepo.Users[pending.ID()].Status() != user.StatusSuspended {
		t.Errorf("user should be suspended")
	}
}

func TestSuspendUserHandler_SelfSuspend(t *testing.T) {
	uRepo := mocks.NewUserRepository()
	tRepo := mocks.NewTenantRepository()
	admin := seedAdminUser(t, uRepo)
	// admin is approved; admin tries to suspend self.
	h := NewSuspendUserHandler(uRepo, tRepo)
	err := h.Handle(context.Background(), SuspendUser{
		UserID:            admin.ID(),
		SuspendedByUserID: admin.ID(),
	})
	if err == nil {
		t.Fatal("expected self-suspend to be blocked")
	}
}

// ================
// ResumeUserHandler
// ================

func TestResumeUserHandler_Success(t *testing.T) {
	uRepo := mocks.NewUserRepository()
	admin := seedAdminUser(t, uRepo)
	pending := seedPendingUser(t, uRepo)
	if err := pending.Approve(admin.ID()); err != nil {
		t.Fatalf("setup Approve: %v", err)
	}
	if err := pending.Suspend(admin.ID(), "x"); err != nil {
		t.Fatalf("setup Suspend: %v", err)
	}
	if err := uRepo.Save(context.Background(), pending); err != nil {
		t.Fatalf("setup Save: %v", err)
	}

	h := NewResumeUserHandler(uRepo)
	if err := h.Handle(context.Background(), ResumeUser{
		UserID:          pending.ID(),
		ResumedByUserID: admin.ID(),
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uRepo.Users[pending.ID()].Status() != user.StatusApproved {
		t.Errorf("user should be approved after resume")
	}
}

func TestResumeUserHandler_AlreadyApproved(t *testing.T) {
	uRepo := mocks.NewUserRepository()
	admin := seedAdminUser(t, uRepo)
	pending := seedPendingUser(t, uRepo)
	if err := pending.Approve(admin.ID()); err != nil {
		t.Fatalf("setup approve: %v", err)
	}
	if err := uRepo.Save(context.Background(), pending); err != nil {
		t.Fatalf("setup save: %v", err)
	}

	h := NewResumeUserHandler(uRepo)
	if err := h.Handle(context.Background(), ResumeUser{
		UserID:          pending.ID(),
		ResumedByUserID: admin.ID(),
	}); err != nil {
		t.Fatalf("idempotent resume should succeed, got %v", err)
	}
}

func TestResumeUserHandler_NotFound(t *testing.T) {
	uRepo := mocks.NewUserRepository()
	h := NewResumeUserHandler(uRepo)
	err := h.Handle(context.Background(), ResumeUser{
		UserID:          "missing",
		ResumedByUserID: "admin",
	})
	if err == nil || !pkgerrors.Is(err, pkgerrors.ErrNotFound) {
		t.Fatalf("expected NotFound, got %v", err)
	}
}

func TestResumeUserHandler_SelfResume(t *testing.T) {
	// Resume has NO self-action guard by design. A solo-admin must be
	// able to recover their own suspended account. This test asserts
	// the documented behavior.
	uRepo := mocks.NewUserRepository()
	admin := seedAdminUser(t, uRepo)
	// Manually suspend admin (bypassing Suspend's self-block) by
	// reconstructing in suspended state. We use a fresh user via the
	// reject path to reach suspended without self-suspend.
	suspended, err := user.RegisterUser("u@e", "google", "g2", false)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := suspended.Reject(admin.ID(), "x"); err != nil {
		t.Fatalf("setup reject: %v", err)
	}
	if err := uRepo.Save(context.Background(), suspended); err != nil {
		t.Fatalf("setup Save: %v", err)
	}

	h := NewResumeUserHandler(uRepo)
	// Self-resume: resumed_by == user_id. Allowed.
	if err := h.Handle(context.Background(), ResumeUser{
		UserID:          suspended.ID(),
		ResumedByUserID: suspended.ID(),
	}); err != nil {
		t.Fatalf("self-resume should succeed, got %v", err)
	}
}

// ===========================
// RetryTenantMigrationHandler
// ===========================

func TestRetryTenantMigrationHandler_Success(t *testing.T) {
	tRepo := mocks.NewTenantRepository()
	pub := mocks.NewEventPublisher()
	te, err := tenant.NewTenant("user-1")
	if err != nil {
		t.Fatalf("setup NewTenant: %v", err)
	}
	if err := te.StartProvisioning(); err != nil {
		t.Fatalf("setup StartProvisioning: %v", err)
	}
	if err := te.MarkProvisioningFailed("migrate failed"); err != nil {
		t.Fatalf("setup MarkProvisioningFailed: %v", err)
	}
	if err := tRepo.Save(context.Background(), te); err != nil {
		t.Fatalf("setup Save: %v", err)
	}

	h := NewRetryTenantMigrationHandler(tRepo, pub)
	if err := h.Handle(context.Background(), RetryTenantMigration{
		TenantID:        te.ID(),
		RetriedByUserID: "admin-1",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tRepo.Tenants[te.ID()].Status() != tenant.StatusProvisioning {
		t.Errorf("expected provisioning, got %s", tRepo.Tenants[te.ID()].Status())
	}
	if pub.Count() != 1 {
		t.Errorf("expected 1 publish, got %d", pub.Count())
	}
}

func TestRetryTenantMigrationHandler_WrongState(t *testing.T) {
	tRepo := mocks.NewTenantRepository()
	pub := mocks.NewEventPublisher()
	te, err := tenant.NewTenant("user-1")
	if err != nil {
		t.Fatalf("setup NewTenant: %v", err)
	}
	if err := te.StartProvisioning(); err != nil {
		t.Fatalf("setup StartProvisioning: %v", err)
	}
	if err := te.Approve("admin-1", 5); err != nil {
		t.Fatalf("setup Approve: %v", err)
	}
	if err := tRepo.Save(context.Background(), te); err != nil {
		t.Fatalf("setup Save: %v", err)
	}

	h := NewRetryTenantMigrationHandler(tRepo, pub)
	err = h.Handle(context.Background(), RetryTenantMigration{
		TenantID:        te.ID(),
		RetriedByUserID: "admin-1",
	})
	if err == nil {
		t.Fatal("expected error retrying an approved tenant")
	}
	if !pkgerrors.Is(err, pkgerrors.ErrInvalidState) {
		t.Errorf("expected InvalidState, got %v", err)
	}
}

func TestRetryTenantMigrationHandler_NotFound(t *testing.T) {
	tRepo := mocks.NewTenantRepository()
	pub := mocks.NewEventPublisher()
	h := NewRetryTenantMigrationHandler(tRepo, pub)
	err := h.Handle(context.Background(), RetryTenantMigration{
		TenantID:        "missing",
		RetriedByUserID: "admin-1",
	})
	if err == nil || !pkgerrors.Is(err, pkgerrors.ErrNotFound) {
		t.Fatalf("expected NotFound, got %v", err)
	}
}

func TestRetryTenantMigrationHandler_AlreadyProvisioning(t *testing.T) {
	tRepo := mocks.NewTenantRepository()
	pub := mocks.NewEventPublisher()
	te, err := tenant.NewTenant("user-1")
	if err != nil {
		t.Fatalf("setup NewTenant: %v", err)
	}
	if err := te.StartProvisioning(); err != nil {
		t.Fatalf("setup StartProvisioning: %v", err)
	}
	if err := tRepo.Save(context.Background(), te); err != nil {
		t.Fatalf("setup Save: %v", err)
	}

	h := NewRetryTenantMigrationHandler(tRepo, pub)
	if err := h.Handle(context.Background(), RetryTenantMigration{
		TenantID:        te.ID(),
		RetriedByUserID: "admin-1",
	}); err != nil {
		t.Fatalf("idempotent retry should succeed, got %v", err)
	}
	if pub.Count() != 1 {
		t.Errorf("should re-publish once, got %d", pub.Count())
	}
}

func TestRetryTenantMigrationHandler_PublishError(t *testing.T) {
	tRepo := mocks.NewTenantRepository()
	pub := mocks.NewEventPublisher()
	pub.PublishFunc = func(ctx context.Context, topic string, payload interface{}) error {
		return errors.New("nats down")
	}
	te, err := tenant.NewTenant("user-1")
	if err != nil {
		t.Fatalf("setup NewTenant: %v", err)
	}
	if err := te.StartProvisioning(); err != nil {
		t.Fatalf("setup StartProvisioning: %v", err)
	}
	if err := te.MarkProvisioningFailed("x"); err != nil {
		t.Fatalf("setup MarkProvisioningFailed: %v", err)
	}
	if err := tRepo.Save(context.Background(), te); err != nil {
		t.Fatalf("setup Save: %v", err)
	}

	h := NewRetryTenantMigrationHandler(tRepo, pub)
	err = h.Handle(context.Background(), RetryTenantMigration{
		TenantID:        te.ID(),
		RetriedByUserID: "admin-1",
	})
	if err == nil {
		t.Fatal("expected error from publish")
	}
	// Tenant Save still happened — by design, the publish failure
	// surfaces but the state machine moved.
	if tRepo.Tenants[te.ID()].Status() != tenant.StatusProvisioning {
		t.Errorf("tenant Save should have happened before publish")
	}
}
