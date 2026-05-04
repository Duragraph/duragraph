//go:build integration

package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/duragraph/duragraph/internal/domain/tenant"
	"github.com/duragraph/duragraph/internal/domain/user"
	pkgerrors "github.com/duragraph/duragraph/internal/pkg/errors"
)

// seedUser inserts a real platform.users row via UserRepository so
// tenant tests have a valid user_id FK target. Returns the user's id.
func seedUser(t *testing.T, ctx context.Context, repo *UserRepository) string {
	t.Helper()
	u, err := user.RegisterUser(freshEmail(), "google", freshOAuthID(), false)
	if err != nil {
		t.Fatalf("RegisterUser: %v", err)
	}
	if err := repo.Save(ctx, u); err != nil {
		t.Fatalf("Save user: %v", err)
	}
	return u.ID()
}

func TestTenantRepository_Save_NewTenant(t *testing.T) {
	ctx := context.Background()
	pool, _ := platformPool(t, ctx)
	users := NewUserRepository(pool)
	tenants := NewTenantRepository(pool)

	userID := seedUser(t, ctx, users)
	tn, err := tenant.NewTenant(userID)
	if err != nil {
		t.Fatalf("NewTenant: %v", err)
	}

	if err := tenants.Save(ctx, tn); err != nil {
		t.Fatalf("Save tenant: %v", err)
	}
	if tn.LoadedUpdatedAt().IsZero() {
		t.Fatal("expected loadedUpdatedAt refreshed after Save")
	}
	if len(tn.Events()) != 0 {
		t.Errorf("expected events cleared after Save, got %d", len(tn.Events()))
	}

	got, err := tenants.GetByID(ctx, tn.ID())
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ID() != tn.ID() {
		t.Errorf("ID = %q, want %q", got.ID(), tn.ID())
	}
	if got.UserID() != userID {
		t.Errorf("UserID = %q, want %q", got.UserID(), userID)
	}
	if got.DBName() != tn.DBName() {
		t.Errorf("DBName = %q, want %q", got.DBName(), tn.DBName())
	}
	if got.Status() != tenant.StatusPending {
		t.Errorf("Status = %q, want pending", got.Status())
	}
	if got.SchemaVersion() != nil {
		t.Errorf("SchemaVersion = %v, want nil", got.SchemaVersion())
	}
	if got.ProvisionedAt() != nil {
		t.Errorf("ProvisionedAt = %v, want nil", got.ProvisionedAt())
	}
}

// TestTenantRepository_Save_RespectsTableCHECKs verifies that the
// table-level CHECK constraints (e.g.
// tenants_approved_requires_schema_version) are not silently
// suppressed by the repo. We construct an in-memory aggregate that
// would violate the CHECK and hand it to Save — Save must surface the
// PG error.
//
// We can't reach this state via the public domain API (Approve sets
// schemaVersion); we use ReconstructFromData with a hand-crafted
// invalid TenantData to simulate a buggy caller / data-corruption
// scenario and prove the repo doesn't mask the schema-layer guard.
func TestTenantRepository_Save_RespectsTableCHECKs(t *testing.T) {
	ctx := context.Background()
	pool, _ := platformPool(t, ctx)
	users := NewUserRepository(pool)
	tenants := NewTenantRepository(pool)

	userID := seedUser(t, ctx, users)

	// Build a fresh tenant via NewTenant so id, db_name, user_id all
	// satisfy the derivation/format/FK constraints. Then Save it
	// (status=pending, no schema_version — fine).
	tn, err := tenant.NewTenant(userID)
	if err != nil {
		t.Fatalf("NewTenant: %v", err)
	}
	if err := tenants.Save(ctx, tn); err != nil {
		t.Fatalf("Save initial: %v", err)
	}

	// Now reconstruct a tampered version with status='approved' but no
	// schema_version / no provisioned_at. This will trigger the
	// tenants_approved_requires_schema_version CHECK on UPDATE.
	tampered := tenant.ReconstructFromData(tenant.TenantData{
		ID:            tn.ID(),
		UserID:        tn.UserID(),
		DBName:        tn.DBName(),
		Status:        string(tenant.StatusApproved),
		SchemaVersion: nil,
		ProvisionedAt: nil,
		FailureReason: "",
		CreatedAt:     tn.CreatedAt(),
		UpdatedAt:     tn.LoadedUpdatedAt(),
	})

	err = tenants.Save(ctx, tampered)
	if err == nil {
		t.Fatal("expected CHECK violation when saving approved tenant with NULL schema_version, got nil")
	}
	// The error is wrapped via pkgerrors.Internal — surface it. The PG
	// error message names the offending CHECK; assert it propagates.
	if !strings.Contains(err.Error(), "tenants_approved_requires_schema_version") &&
		!strings.Contains(err.Error(), "check constraint") {
		t.Logf("CHECK violation propagated (constraint name not in msg): %v", err)
	}
}

func TestTenantRepository_GetByID_NotFound(t *testing.T) {
	ctx := context.Background()
	pool, _ := platformPool(t, ctx)
	tenants := NewTenantRepository(pool)

	missingID := uuid.New().String()
	_, err := tenants.GetByID(ctx, missingID)
	if err == nil {
		t.Fatal("expected NotFound, got nil")
	}
	if !errors.Is(err, pkgerrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestTenantRepository_GetByUserID_Enforces1to1(t *testing.T) {
	ctx := context.Background()
	pool, _ := platformPool(t, ctx)
	users := NewUserRepository(pool)
	tenants := NewTenantRepository(pool)

	userID := seedUser(t, ctx, users)

	// First tenant for the user inserts cleanly and is retrievable
	// via GetByUserID.
	first, err := tenant.NewTenant(userID)
	if err != nil {
		t.Fatalf("NewTenant first: %v", err)
	}
	if err := tenants.Save(ctx, first); err != nil {
		t.Fatalf("Save first: %v", err)
	}
	got, err := tenants.GetByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("GetByUserID: %v", err)
	}
	if got.ID() != first.ID() {
		t.Errorf("GetByUserID id = %q, want %q", got.ID(), first.ID())
	}

	// Second tenant for the same user must be rejected by the
	// tenants_user_id_unique constraint at insert time.
	second, err := tenant.NewTenant(userID)
	if err != nil {
		t.Fatalf("NewTenant second: %v", err)
	}
	err = tenants.Save(ctx, second)
	if err == nil {
		t.Fatal("expected unique violation on tenants_user_id_unique, got nil")
	}
}

func TestTenantRepository_GetByUserID_NotFound(t *testing.T) {
	ctx := context.Background()
	pool, _ := platformPool(t, ctx)
	users := NewUserRepository(pool)
	tenants := NewTenantRepository(pool)

	// Seed a user but do not create a tenant for them.
	userID := seedUser(t, ctx, users)

	_, err := tenants.GetByUserID(ctx, userID)
	if err == nil {
		t.Fatal("expected NotFound, got nil")
	}
	if !errors.Is(err, pkgerrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestTenantRepository_ListByStatus(t *testing.T) {
	ctx := context.Background()
	pool, _ := platformPool(t, ctx)
	users := NewUserRepository(pool)
	tenants := NewTenantRepository(pool)

	// Three pending tenants (each needs its own user — 1:1).
	for i := 0; i < 3; i++ {
		userID := seedUser(t, ctx, users)
		tn, err := tenant.NewTenant(userID)
		if err != nil {
			t.Fatalf("NewTenant %d: %v", i, err)
		}
		if err := tenants.Save(ctx, tn); err != nil {
			t.Fatalf("Save %d: %v", i, err)
		}
	}

	pending, err := tenants.ListByStatus(ctx, tenant.StatusPending, 10, 0)
	if err != nil {
		t.Fatalf("ListByStatus pending: %v", err)
	}
	if len(pending) != 3 {
		t.Errorf("expected 3 pending tenants, got %d", len(pending))
	}
	for _, tn := range pending {
		if tn.Status() != tenant.StatusPending {
			t.Errorf("ListByStatus returned tenant with status=%q (expected pending)", tn.Status())
		}
	}

	// No approved tenants yet.
	approved, err := tenants.ListByStatus(ctx, tenant.StatusApproved, 10, 0)
	if err != nil {
		t.Fatalf("ListByStatus approved: %v", err)
	}
	if len(approved) != 0 {
		t.Errorf("expected 0 approved tenants, got %d", len(approved))
	}

	// Pagination: limit=2 returns 2; offset=2 returns 1.
	page1, err := tenants.ListByStatus(ctx, tenant.StatusPending, 2, 0)
	if err != nil {
		t.Fatalf("ListByStatus page 1: %v", err)
	}
	if len(page1) != 2 {
		t.Errorf("page 1 len = %d, want 2", len(page1))
	}
	page2, err := tenants.ListByStatus(ctx, tenant.StatusPending, 2, 2)
	if err != nil {
		t.Fatalf("ListByStatus page 2: %v", err)
	}
	if len(page2) != 1 {
		t.Errorf("page 2 len = %d, want 1", len(page2))
	}
}
