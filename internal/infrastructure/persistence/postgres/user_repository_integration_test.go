//go:build integration

package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/duragraph/duragraph/internal/domain/user"
	pkgerrors "github.com/duragraph/duragraph/internal/pkg/errors"
)

// platformPool returns a fresh *pgxpool.Pool connected to a freshly
// bootstrapped per-test platform DB. The pool is registered with
// t.Cleanup for automatic close. Combines the testcontainer DSN
// (sharedContainer) with newMigratorForTest (which generates a unique
// platform DB name and runs Bootstrap to apply platform/* migrations).
//
// Shared by user_repository_integration_test.go and
// tenant_repository_integration_test.go.
func platformPool(t *testing.T, ctx context.Context) (*pgxpool.Pool, string) {
	t.Helper()
	m, platformDB := newMigratorForTest(t)
	if err := m.Bootstrap(ctx); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	_, dsn := sharedContainer(t)
	pool, err := pgxpool.New(ctx, adminURLForDB(t, dsn, platformDB))
	if err != nil {
		t.Fatalf("open platform pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool, platformDB
}

// freshOAuthID returns a unique synthetic oauth_id for a test user.
func freshOAuthID() string {
	return "oauth-" + uuid.New().String()
}

// freshEmail returns a unique synthetic email so tests can run in
// parallel without colliding on the email UNIQUE constraint.
func freshEmail() string {
	return uuid.New().String() + "@example.com"
}

func TestUserRepository_Save_NewUser(t *testing.T) {
	ctx := context.Background()
	pool, _ := platformPool(t, ctx)
	repo := NewUserRepository(pool)

	u, err := user.RegisterUser(freshEmail(), "google", freshOAuthID(), false)
	if err != nil {
		t.Fatalf("RegisterUser: %v", err)
	}

	if err := repo.Save(ctx, u); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if u.LoadedUpdatedAt().IsZero() {
		t.Fatal("expected loadedUpdatedAt to be refreshed after Save")
	}
	if len(u.Events()) != 0 {
		t.Errorf("expected events cleared after Save, got %d", len(u.Events()))
	}

	got, err := repo.GetByID(ctx, u.ID())
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ID() != u.ID() {
		t.Errorf("ID = %q, want %q", got.ID(), u.ID())
	}
	if got.Email() != u.Email() {
		t.Errorf("Email = %q, want %q", got.Email(), u.Email())
	}
	if got.Status() != user.StatusPending {
		t.Errorf("Status = %q, want pending", got.Status())
	}
	if got.Role() != user.RoleUser {
		t.Errorf("Role = %q, want user", got.Role())
	}
	if got.LoadedUpdatedAt().IsZero() {
		t.Error("loaded user should carry non-zero loadedUpdatedAt")
	}
}

func TestUserRepository_Save_UpsertExisting(t *testing.T) {
	ctx := context.Background()
	pool, _ := platformPool(t, ctx)
	repo := NewUserRepository(pool)

	// Bootstrap path: emit the admin user that becomes "the actor"
	// approving our pending user. Persist it then forget it (we don't
	// need the pointer back).
	admin, err := user.RegisterUser(freshEmail(), "google", freshOAuthID(), true)
	if err != nil {
		t.Fatalf("RegisterUser admin: %v", err)
	}
	if err := repo.Save(ctx, admin); err != nil {
		t.Fatalf("Save admin: %v", err)
	}

	target, err := user.RegisterUser(freshEmail(), "google", freshOAuthID(), false)
	if err != nil {
		t.Fatalf("RegisterUser target: %v", err)
	}
	if err := repo.Save(ctx, target); err != nil {
		t.Fatalf("Save target: %v", err)
	}

	// Reload, mutate (Approve), Save again.
	loaded, err := repo.GetByID(ctx, target.ID())
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if err := loaded.Approve(admin.ID()); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if err := repo.Save(ctx, loaded); err != nil {
		t.Fatalf("Save (update): %v", err)
	}
	// loaded started fresh from GetByID with version=1 and a successful
	// Save bumps the in-memory soft counter to 2.
	if loaded.Version() < 2 {
		t.Errorf("expected version >= 2 after upsert, got %d", loaded.Version())
	}

	got, err := repo.GetByID(ctx, target.ID())
	if err != nil {
		t.Fatalf("GetByID after update: %v", err)
	}
	if got.Status() != user.StatusApproved {
		t.Errorf("Status after update = %q, want approved", got.Status())
	}
}

func TestUserRepository_Save_OptimisticConcurrencyConflict(t *testing.T) {
	ctx := context.Background()
	pool, _ := platformPool(t, ctx)
	repo := NewUserRepository(pool)

	// Seed admin to approve with.
	admin, err := user.RegisterUser(freshEmail(), "google", freshOAuthID(), true)
	if err != nil {
		t.Fatalf("RegisterUser admin: %v", err)
	}
	if err := repo.Save(ctx, admin); err != nil {
		t.Fatalf("Save admin: %v", err)
	}

	target, err := user.RegisterUser(freshEmail(), "google", freshOAuthID(), false)
	if err != nil {
		t.Fatalf("RegisterUser target: %v", err)
	}
	if err := repo.Save(ctx, target); err != nil {
		t.Fatalf("Save target: %v", err)
	}

	// Two independent loads — each carries the same loadedUpdatedAt.
	first, err := repo.GetByID(ctx, target.ID())
	if err != nil {
		t.Fatalf("GetByID first: %v", err)
	}
	second, err := repo.GetByID(ctx, target.ID())
	if err != nil {
		t.Fatalf("GetByID second: %v", err)
	}

	// Both mutate independently.
	if err := first.Approve(admin.ID()); err != nil {
		t.Fatalf("Approve first: %v", err)
	}
	if err := second.Approve(admin.ID()); err != nil {
		t.Fatalf("Approve second: %v", err)
	}

	// First Save wins.
	if err := repo.Save(ctx, first); err != nil {
		t.Fatalf("Save first (should win): %v", err)
	}

	// Second Save should fail with ErrConcurrency.
	err = repo.Save(ctx, second)
	if err == nil {
		t.Fatal("expected concurrency conflict error, got nil")
	}
	if !errors.Is(err, pkgerrors.ErrConcurrency) {
		t.Errorf("expected ErrConcurrency, got: %v", err)
	}
}

func TestUserRepository_GetByID_NotFound(t *testing.T) {
	ctx := context.Background()
	pool, _ := platformPool(t, ctx)
	repo := NewUserRepository(pool)

	missingID := uuid.New().String()
	_, err := repo.GetByID(ctx, missingID)
	if err == nil {
		t.Fatal("expected NotFound, got nil")
	}
	if !errors.Is(err, pkgerrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestUserRepository_GetByOAuth_RoundTrip(t *testing.T) {
	ctx := context.Background()
	pool, _ := platformPool(t, ctx)
	repo := NewUserRepository(pool)

	provider := "github"
	oauthID := freshOAuthID()
	u, err := user.RegisterUser(freshEmail(), provider, oauthID, false)
	if err != nil {
		t.Fatalf("RegisterUser: %v", err)
	}
	if err := repo.Save(ctx, u); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := repo.GetByOAuth(ctx, provider, oauthID)
	if err != nil {
		t.Fatalf("GetByOAuth: %v", err)
	}
	if got.ID() != u.ID() {
		t.Errorf("GetByOAuth returned id=%q, want %q", got.ID(), u.ID())
	}
	if got.OAuthProvider() != provider {
		t.Errorf("OAuthProvider = %q, want %q", got.OAuthProvider(), provider)
	}
	if got.OAuthID() != oauthID {
		t.Errorf("OAuthID = %q, want %q", got.OAuthID(), oauthID)
	}
}

func TestUserRepository_GetByOAuth_NotFound(t *testing.T) {
	ctx := context.Background()
	pool, _ := platformPool(t, ctx)
	repo := NewUserRepository(pool)

	_, err := repo.GetByOAuth(ctx, "google", "no-such-oauth-id")
	if err == nil {
		t.Fatal("expected NotFound, got nil")
	}
	if !errors.Is(err, pkgerrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestUserRepository_ListByStatus_FiltersAndPaginates(t *testing.T) {
	ctx := context.Background()
	pool, _ := platformPool(t, ctx)
	repo := NewUserRepository(pool)

	// Bootstrap admin (approved + admin).
	admin, err := user.RegisterUser(freshEmail(), "google", freshOAuthID(), true)
	if err != nil {
		t.Fatalf("RegisterUser admin: %v", err)
	}
	if err := repo.Save(ctx, admin); err != nil {
		t.Fatalf("Save admin: %v", err)
	}

	// Create three pending users.
	pendingIDs := make(map[string]bool)
	for i := 0; i < 3; i++ {
		p, err := user.RegisterUser(freshEmail(), "google", freshOAuthID(), false)
		if err != nil {
			t.Fatalf("RegisterUser pending: %v", err)
		}
		if err := repo.Save(ctx, p); err != nil {
			t.Fatalf("Save pending: %v", err)
		}
		pendingIDs[p.ID()] = true
	}

	// List pending — should return all 3 (admin is approved, not pending).
	pending, err := repo.ListByStatus(ctx, user.StatusPending, 10, 0)
	if err != nil {
		t.Fatalf("ListByStatus pending: %v", err)
	}
	if len(pending) != 3 {
		t.Errorf("expected 3 pending users, got %d", len(pending))
	}
	for _, p := range pending {
		if !pendingIDs[p.ID()] {
			t.Errorf("ListByStatus returned unexpected user id %s", p.ID())
		}
		if p.Status() != user.StatusPending {
			t.Errorf("ListByStatus returned user with status=%q (expected pending)", p.Status())
		}
	}

	// List approved — should return only the admin.
	approved, err := repo.ListByStatus(ctx, user.StatusApproved, 10, 0)
	if err != nil {
		t.Fatalf("ListByStatus approved: %v", err)
	}
	if len(approved) != 1 {
		t.Fatalf("expected 1 approved user, got %d", len(approved))
	}
	if approved[0].ID() != admin.ID() {
		t.Errorf("approved[0].ID = %q, want %q", approved[0].ID(), admin.ID())
	}

	// Pagination: limit=2, offset=0 then offset=2.
	page1, err := repo.ListByStatus(ctx, user.StatusPending, 2, 0)
	if err != nil {
		t.Fatalf("ListByStatus pending page 1: %v", err)
	}
	if len(page1) != 2 {
		t.Errorf("page 1 len = %d, want 2", len(page1))
	}
	page2, err := repo.ListByStatus(ctx, user.StatusPending, 2, 2)
	if err != nil {
		t.Fatalf("ListByStatus pending page 2: %v", err)
	}
	if len(page2) != 1 {
		t.Errorf("page 2 len = %d, want 1", len(page2))
	}
}

func TestUserRepository_CountAll(t *testing.T) {
	ctx := context.Background()
	pool, _ := platformPool(t, ctx)
	repo := NewUserRepository(pool)

	// Initially zero.
	count, err := repo.CountAll(ctx)
	if err != nil {
		t.Fatalf("CountAll initial: %v", err)
	}
	if count != 0 {
		t.Errorf("initial CountAll = %d, want 0", count)
	}

	// Insert two users; CountAll bumps to 2.
	for i := 0; i < 2; i++ {
		u, err := user.RegisterUser(freshEmail(), "google", freshOAuthID(), false)
		if err != nil {
			t.Fatalf("RegisterUser %d: %v", i, err)
		}
		if err := repo.Save(ctx, u); err != nil {
			t.Fatalf("Save %d: %v", i, err)
		}
	}

	count, err = repo.CountAll(ctx)
	if err != nil {
		t.Fatalf("CountAll after inserts: %v", err)
	}
	if count != 2 {
		t.Errorf("CountAll after inserts = %d, want 2", count)
	}
}
