//go:build integration

package postgres

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// Shared testcontainer is brought up once per `go test` invocation.
// We deliberately do NOT register teardown here: t.Cleanup is per-
// test, and registering on the first test would terminate the
// container before subsequent tests run. The existing
// integration_test.go in the external _test package already owns the
// binary's only TestMain, so we can't add one here. The testcontainers
// Ryuk reaper handles container cleanup at process exit; the admin
// pool is reclaimed when the test binary terminates.
var (
	containerOnce sync.Once
	containerErr  error
	sharedAdmin   *pgxpool.Pool
	sharedDSN     string
)

// bootstrapContainer spins up Postgres via testcontainers-go and
// stores admin pool + DSN for reuse. Called via sync.Once so the
// container survives across tests in this binary.
func bootstrapContainer(ctx context.Context) error {
	c, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("admin"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		return fmt.Errorf("start postgres container: %w", err)
	}

	dsn, err := c.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return fmt.Errorf("get connection string: %w", err)
	}
	sharedDSN = dsn

	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("open admin pool: %w", err)
	}
	if err := admin.Ping(ctx); err != nil {
		return fmt.Errorf("ping admin pool: %w", err)
	}
	sharedAdmin = admin
	return nil
}

func sharedContainer(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()
	ctx := context.Background()
	containerOnce.Do(func() {
		containerErr = bootstrapContainer(ctx)
	})
	if containerErr != nil {
		t.Fatalf("testcontainer bootstrap failed: %v", containerErr)
	}
	return sharedAdmin, sharedDSN
}

// freshTenantUUID generates a UUID v4 for use as a tenant id and
// returns both the canonical string form and the derived db name.
func freshTenantUUID(t *testing.T) (string, string) {
	t.Helper()
	id := uuid.New().String()
	dbName := "tenant_" + strings.ReplaceAll(id, "-", "")
	return id, dbName
}

// createTenantDB issues `CREATE DATABASE` against the testcontainer's
// admin pool. dbName is interpolated; callers must pass already-
// validated names (the helper here only takes derived strings).
func createTenantDB(t *testing.T, ctx context.Context, admin *pgxpool.Pool, dbName string) {
	t.Helper()
	if _, err := admin.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s", dbName)); err != nil {
		t.Fatalf("create database %s: %v", dbName, err)
	}
}

// newManagerForDSN parses the testcontainer DSN into a base
// pgxpool.Config and returns a fresh manager. The Database field on
// the base config is irrelevant — PoolManager overwrites it per
// tenant.
func newManagerForDSN(t *testing.T, dsn string, opts ...Option) *PoolManager {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse base config: %v", err)
	}
	return NewPoolManager(cfg, opts...)
}

func TestForTenant_LazyCreation(t *testing.T) {
	ctx := context.Background()
	admin, dsn := sharedContainer(t)

	tenantID, dbName := freshTenantUUID(t)
	createTenantDB(t, ctx, admin, dbName)

	m := newManagerForDSN(t, dsn)
	defer func() { _ = m.Close() }()

	pool, err := m.ForTenant(ctx, tenantID)
	if err != nil {
		t.Fatalf("ForTenant: %v", err)
	}
	if pool == nil {
		t.Fatal("ForTenant returned nil pool")
	}

	var got int
	if err := pool.QueryRow(ctx, "SELECT 1").Scan(&got); err != nil {
		t.Fatalf("SELECT 1: %v", err)
	}
	if got != 1 {
		t.Fatalf("SELECT 1 returned %d", got)
	}
}

func TestForTenant_CachedAcrossCalls(t *testing.T) {
	ctx := context.Background()
	admin, dsn := sharedContainer(t)

	tenantID, dbName := freshTenantUUID(t)
	createTenantDB(t, ctx, admin, dbName)

	m := newManagerForDSN(t, dsn)
	defer func() { _ = m.Close() }()

	first, err := m.ForTenant(ctx, tenantID)
	if err != nil {
		t.Fatalf("first ForTenant: %v", err)
	}
	second, err := m.ForTenant(ctx, tenantID)
	if err != nil {
		t.Fatalf("second ForTenant: %v", err)
	}
	if first != second {
		t.Fatalf("expected cached pool, got distinct pointers (%p vs %p)", first, second)
	}
}

func TestForTenant_IsolatedAcrossTenants(t *testing.T) {
	ctx := context.Background()
	admin, dsn := sharedContainer(t)

	tenantA, dbA := freshTenantUUID(t)
	tenantB, dbB := freshTenantUUID(t)
	createTenantDB(t, ctx, admin, dbA)
	createTenantDB(t, ctx, admin, dbB)

	m := newManagerForDSN(t, dsn)
	defer func() { _ = m.Close() }()

	poolA, err := m.ForTenant(ctx, tenantA)
	if err != nil {
		t.Fatalf("ForTenant A: %v", err)
	}
	poolB, err := m.ForTenant(ctx, tenantB)
	if err != nil {
		t.Fatalf("ForTenant B: %v", err)
	}
	if poolA == poolB {
		t.Fatal("expected distinct pools for distinct tenants")
	}

	var nameA, nameB string
	if err := poolA.QueryRow(ctx, "SELECT current_database()").Scan(&nameA); err != nil {
		t.Fatalf("query A: %v", err)
	}
	if err := poolB.QueryRow(ctx, "SELECT current_database()").Scan(&nameB); err != nil {
		t.Fatalf("query B: %v", err)
	}
	if nameA != dbA {
		t.Errorf("pool A current_database() = %q, want %q", nameA, dbA)
	}
	if nameB != dbB {
		t.Errorf("pool B current_database() = %q, want %q", nameB, dbB)
	}
}

func TestForTenant_ConcurrentSafe(t *testing.T) {
	ctx := context.Background()
	admin, dsn := sharedContainer(t)

	const tenantCount = 10
	const goroutines = 50

	tenantIDs := make([]string, tenantCount)
	for i := 0; i < tenantCount; i++ {
		id, dbName := freshTenantUUID(t)
		tenantIDs[i] = id
		createTenantDB(t, ctx, admin, dbName)
	}

	m := newManagerForDSN(t, dsn)
	defer func() { _ = m.Close() }()

	var wg sync.WaitGroup
	errCh := make(chan error, goroutines)

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			r := rand.New(rand.NewSource(int64(seed) + time.Now().UnixNano()))
			for i := 0; i < tenantCount; i++ {
				idx := r.Intn(tenantCount)
				if _, err := m.ForTenant(ctx, tenantIDs[idx]); err != nil {
					errCh <- err
					return
				}
			}
		}(g)
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("concurrent ForTenant: %v", err)
	}

	m.mu.RLock()
	got := len(m.pools)
	m.mu.RUnlock()
	if got != tenantCount {
		t.Errorf("pool count after concurrent access = %d, want %d", got, tenantCount)
	}
}

func TestIdleEviction(t *testing.T) {
	ctx := context.Background()
	admin, dsn := sharedContainer(t)

	tenantID, dbName := freshTenantUUID(t)
	createTenantDB(t, ctx, admin, dbName)

	m := newManagerForDSN(t, dsn,
		WithIdleTimeout(200*time.Millisecond),
		WithEvictInterval(100*time.Millisecond),
	)
	m.Start(ctx)
	defer func() { _ = m.Close() }()

	first, err := m.ForTenant(ctx, tenantID)
	if err != nil {
		t.Fatalf("first ForTenant: %v", err)
	}

	// Sleep > idleTimeout WITHOUT touching the manager so lastUsed
	// stays stale and the eviction goroutine has multiple ticks to
	// fire.
	time.Sleep(500 * time.Millisecond)

	second, err := m.ForTenant(ctx, tenantID)
	if err != nil {
		t.Fatalf("second ForTenant: %v", err)
	}
	if first == second {
		t.Fatal("expected eviction to discard the first pool, but got the same pointer")
	}
}

func TestClose_EvictsAll(t *testing.T) {
	ctx := context.Background()
	admin, dsn := sharedContainer(t)

	const n = 3
	pools := make([]*pgxpool.Pool, n)
	tenantIDs := make([]string, n)
	for i := 0; i < n; i++ {
		id, dbName := freshTenantUUID(t)
		tenantIDs[i] = id
		createTenantDB(t, ctx, admin, dbName)
	}

	m := newManagerForDSN(t, dsn)
	for i, id := range tenantIDs {
		p, err := m.ForTenant(ctx, id)
		if err != nil {
			t.Fatalf("ForTenant[%d]: %v", i, err)
		}
		pools[i] = p
	}

	if err := m.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// After Close, the captured pools should be unusable. pgxpool's
	// Acquire returns an error on a closed pool.
	for i, p := range pools {
		conn, err := p.Acquire(ctx)
		if err == nil {
			conn.Release()
			t.Errorf("pool[%d] Acquire succeeded after Close, expected error", i)
		}
	}

	// And the manager itself should reject further ForTenant calls.
	if _, err := m.ForTenant(ctx, tenantIDs[0]); err == nil {
		t.Error("ForTenant after Close: expected error, got nil")
	}
}

func TestValidateTenantDBName(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "canonical 32-hex lowercase",
			input:   "tenant_0123456789abcdef0123456789abcdef",
			wantErr: false,
		},
		{
			name:    "all zeros",
			input:   "tenant_00000000000000000000000000000000",
			wantErr: false,
		},
		{
			name:    "wrong prefix",
			input:   "tenants_0123456789abcdef0123456789abcdef",
			wantErr: true,
		},
		{
			name:    "no prefix",
			input:   "0123456789abcdef0123456789abcdef",
			wantErr: true,
		},
		{
			name:    "uppercase hex",
			input:   "tenant_0123456789ABCDEF0123456789ABCDEF",
			wantErr: true,
		},
		{
			name:    "too short",
			input:   "tenant_0123456789abcdef0123456789abcde",
			wantErr: true,
		},
		{
			name:    "too long",
			input:   "tenant_0123456789abcdef0123456789abcdef0",
			wantErr: true,
		},
		{
			name:    "non-hex character",
			input:   "tenant_0123456789abcdef0123456789abcdeg",
			wantErr: true,
		},
		{
			name:    "embedded hyphens (un-stripped uuid)",
			input:   "tenant_01234567-89ab-cdef-0123-456789abcdef",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateTenantDBName(tc.input)
			gotErr := err != nil
			if gotErr != tc.wantErr {
				t.Fatalf("validateTenantDBName(%q) err=%v, wantErr=%v", tc.input, err, tc.wantErr)
			}
		})
	}
}
