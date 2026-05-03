//go:build integration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/duragraph/duragraph/internal/domain/tenant"
)

// adminURLForDB rewrites the testcontainer DSN to point at dbName.
// Used by tests that need to inspect a specific DB outside the
// migrator (e.g. counting rows in a tenant DB to verify migrations).
func adminURLForDB(t *testing.T, baseDSN, dbName string) string {
	t.Helper()
	// sharedDSN is `postgres://postgres:postgres@host:port/admin?sslmode=disable`.
	// Replace the path component (between "@.../" and "?...") with dbName.
	at := strings.LastIndex(baseDSN, "@")
	if at < 0 {
		t.Fatalf("dsn missing @: %s", baseDSN)
	}
	rest := baseDSN[at+1:]
	slash := strings.Index(rest, "/")
	if slash < 0 {
		t.Fatalf("dsn missing path: %s", baseDSN)
	}
	q := strings.Index(rest[slash:], "?")
	tail := ""
	if q >= 0 {
		tail = rest[slash:][q:]
	}
	return baseDSN[:at+1] + rest[:slash] + "/" + dbName + tail
}

// dbExists checks pg_database for dbName. Uses an admin pool against
// the testcontainer.
func dbExists(t *testing.T, ctx context.Context, dbName string) bool {
	t.Helper()
	admin, _ := sharedContainer(t)
	var exists bool
	if err := admin.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)`, dbName,
	).Scan(&exists); err != nil {
		t.Fatalf("dbExists check: %v", err)
	}
	return exists
}

// tableExists connects directly to dbName (not through the migrator)
// and checks information_schema for tableName. Used to verify that
// tenant migrations actually created the expected schema objects.
func tableExists(t *testing.T, ctx context.Context, dbName, tableName string) bool {
	t.Helper()
	_, dsn := sharedContainer(t)
	conn, err := pgx.Connect(ctx, adminURLForDB(t, dsn, dbName))
	if err != nil {
		t.Fatalf("connect %s: %v", dbName, err)
	}
	defer conn.Close(ctx)

	var exists bool
	if err := conn.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema='public' AND table_name=$1
		)`, tableName,
	).Scan(&exists); err != nil {
		t.Fatalf("check table %s.%s: %v", dbName, tableName, err)
	}
	return exists
}

// migrationVersion connects directly to dbName and returns the
// schema_migrations.version value. Used to verify idempotency: a
// second Up() call must not change this.
func migrationVersion(t *testing.T, ctx context.Context, dbName string) (uint, bool) {
	t.Helper()
	_, dsn := sharedContainer(t)
	conn, err := pgx.Connect(ctx, adminURLForDB(t, dsn, dbName))
	if err != nil {
		t.Fatalf("connect %s: %v", dbName, err)
	}
	defer conn.Close(ctx)

	var version uint
	var dirty bool
	err = conn.QueryRow(ctx, `SELECT version, dirty FROM schema_migrations`).Scan(&version, &dirty)
	if err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	return version, dirty
}

// dropDatabase nukes a DB so tests can be re-run / isolated.
func dropDatabase(t *testing.T, ctx context.Context, dbName string) {
	t.Helper()
	admin, _ := sharedContainer(t)
	if _, err := admin.Exec(ctx, fmt.Sprintf(`DROP DATABASE IF EXISTS %s`, dbName)); err != nil {
		t.Fatalf("drop database %s: %v", dbName, err)
	}
}

// freshPlatformDBName produces a unique platform DB name per test so
// tests can run in parallel without colliding on the singleton.
func freshPlatformDBName(t *testing.T) string {
	t.Helper()
	return "platform_" + strings.ReplaceAll(uuid.New().String(), "-", "")
}

// newMigratorForTest constructs a Migrator pointing at the shared
// testcontainer, with a unique platform DB name so each test owns its
// own platform DB.
func newMigratorForTest(t *testing.T) (*Migrator, string) {
	t.Helper()
	_, dsn := sharedContainer(t)
	platformDB := freshPlatformDBName(t)
	m, err := NewMigrator(dsn, WithPlatformDBName(platformDB))
	if err != nil {
		t.Fatalf("NewMigrator: %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		dropDatabase(t, ctx, platformDB)
	})
	return m, platformDB
}

// TestMigrator_NewMigrator_DefaultPlatformDBName is the unit-test smoke
// for the option pattern. No testcontainer needed; just confirms the
// default is "duragraph_platform" and overriding sticks.
func TestMigrator_NewMigrator_DefaultPlatformDBName(t *testing.T) {
	m, err := NewMigrator("postgres://u:p@h:5432/postgres?sslmode=disable")
	if err != nil {
		t.Fatalf("NewMigrator: %v", err)
	}
	if m.platformDB != "duragraph_platform" {
		t.Errorf("default platform db = %q, want %q", m.platformDB, "duragraph_platform")
	}

	m2, err := NewMigrator("postgres://u:p@h:5432/postgres?sslmode=disable",
		WithPlatformDBName("custom_platform"))
	if err != nil {
		t.Fatalf("NewMigrator with override: %v", err)
	}
	if m2.platformDB != "custom_platform" {
		t.Errorf("override platform db = %q, want %q", m2.platformDB, "custom_platform")
	}
}

func TestMigrator_Bootstrap_CreatesPlatformDB(t *testing.T) {
	ctx := context.Background()
	m, platformDB := newMigratorForTest(t)

	if dbExists(t, ctx, platformDB) {
		t.Fatalf("expected platform DB %s to be absent before bootstrap", platformDB)
	}

	if err := m.Bootstrap(ctx); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if !dbExists(t, ctx, platformDB) {
		t.Fatalf("expected platform DB %s to exist after bootstrap", platformDB)
	}
}

// TestMigrator_Bootstrap_HandlesEmptyPlatformMigrationsDir is the most
// load-bearing test in this PR: the platform/ embed dir is empty (just
// a README) until feat/platform-db-init lands. Bootstrap must succeed
// in that state.
func TestMigrator_Bootstrap_HandlesEmptyPlatformMigrationsDir(t *testing.T) {
	ctx := context.Background()
	m, platformDB := newMigratorForTest(t)

	// First call creates the DB and runs MigratePlatform with empty FS.
	// Should NOT return an error despite no migrations being applied.
	if err := m.Bootstrap(ctx); err != nil {
		t.Fatalf("Bootstrap with empty platform migrations: %v", err)
	}
	if !dbExists(t, ctx, platformDB) {
		t.Fatalf("expected platform DB %s to exist", platformDB)
	}
}

func TestMigrator_Bootstrap_Idempotent(t *testing.T) {
	ctx := context.Background()
	m, platformDB := newMigratorForTest(t)

	if err := m.Bootstrap(ctx); err != nil {
		t.Fatalf("first Bootstrap: %v", err)
	}
	if err := m.Bootstrap(ctx); err != nil {
		t.Fatalf("second Bootstrap: %v", err)
	}
	if !dbExists(t, ctx, platformDB) {
		t.Fatalf("expected platform DB %s to exist after idempotent bootstrap", platformDB)
	}
}

func TestMigrator_MigrateMainDB_AppliesTenantMigrations(t *testing.T) {
	ctx := context.Background()
	m, _ := newMigratorForTest(t)

	mainDB := "main_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	t.Cleanup(func() { dropDatabase(t, context.Background(), mainDB) })

	// Pre-create the main DB the way docker-compose's POSTGRES_DB env
	// would. The migrator doesn't create the main DB itself (that's
	// the postgres image's job).
	admin, _ := sharedContainer(t)
	if _, err := admin.Exec(ctx, fmt.Sprintf(`CREATE DATABASE %s`, mainDB)); err != nil {
		t.Fatalf("create main DB: %v", err)
	}

	if err := m.MigrateMainDB(ctx, mainDB); err != nil {
		t.Fatalf("MigrateMainDB: %v", err)
	}

	// Spot-check tables from the tenant migrations are present.
	for _, table := range []string{"runs", "assistants", "threads", "events", "outbox"} {
		if !tableExists(t, ctx, mainDB, table) {
			t.Errorf("expected table %s in %s, not found", table, mainDB)
		}
	}
}

func TestMigrator_MigrateMainDB_Idempotent(t *testing.T) {
	ctx := context.Background()
	m, _ := newMigratorForTest(t)

	mainDB := "main_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	t.Cleanup(func() { dropDatabase(t, context.Background(), mainDB) })

	admin, _ := sharedContainer(t)
	if _, err := admin.Exec(ctx, fmt.Sprintf(`CREATE DATABASE %s`, mainDB)); err != nil {
		t.Fatalf("create main DB: %v", err)
	}

	if err := m.MigrateMainDB(ctx, mainDB); err != nil {
		t.Fatalf("first MigrateMainDB: %v", err)
	}
	v1, dirty := migrationVersion(t, ctx, mainDB)
	if dirty {
		t.Fatalf("schema_migrations dirty=true after first run")
	}

	if err := m.MigrateMainDB(ctx, mainDB); err != nil {
		t.Fatalf("second MigrateMainDB: %v", err)
	}
	v2, dirty := migrationVersion(t, ctx, mainDB)
	if dirty {
		t.Fatalf("schema_migrations dirty=true after second run")
	}
	if v1 != v2 {
		t.Errorf("version changed across idempotent runs: v1=%d v2=%d", v1, v2)
	}
}

func TestMigrator_ProvisionTenant_CreatesDBAndApplies(t *testing.T) {
	ctx := context.Background()
	m, _ := newMigratorForTest(t)

	tenantID := uuid.New().String()
	dbName, err := tenant.DBName(tenantID)
	if err != nil {
		t.Fatalf("tenant.DBName: %v", err)
	}
	t.Cleanup(func() { dropDatabase(t, context.Background(), dbName) })

	if dbExists(t, ctx, dbName) {
		t.Fatalf("expected tenant DB %s to be absent before provision", dbName)
	}
	if err := m.ProvisionTenant(ctx, tenantID); err != nil {
		t.Fatalf("ProvisionTenant: %v", err)
	}
	if !dbExists(t, ctx, dbName) {
		t.Fatalf("expected tenant DB %s after provision", dbName)
	}

	for _, table := range []string{"runs", "assistants", "threads"} {
		if !tableExists(t, ctx, dbName, table) {
			t.Errorf("expected table %s in tenant DB %s", table, dbName)
		}
	}
}

func TestMigrator_ProvisionTenant_Idempotent(t *testing.T) {
	ctx := context.Background()
	m, _ := newMigratorForTest(t)

	tenantID := uuid.New().String()
	dbName, err := tenant.DBName(tenantID)
	if err != nil {
		t.Fatalf("tenant.DBName: %v", err)
	}
	t.Cleanup(func() { dropDatabase(t, context.Background(), dbName) })

	if err := m.ProvisionTenant(ctx, tenantID); err != nil {
		t.Fatalf("first ProvisionTenant: %v", err)
	}
	v1, _ := migrationVersion(t, ctx, dbName)

	if err := m.ProvisionTenant(ctx, tenantID); err != nil {
		t.Fatalf("second ProvisionTenant: %v", err)
	}
	v2, _ := migrationVersion(t, ctx, dbName)

	if v1 != v2 {
		t.Errorf("version changed across idempotent provisions: v1=%d v2=%d", v1, v2)
	}
}

// TestMigrator_MigrateAllTenants_EmptyWhenNoTenantsTable verifies the
// graceful-empty path: when the platform DB exists but the
// platform.tenants table doesn't (no platform migrations applied),
// MigrateAllTenants returns no results without error.
func TestMigrator_MigrateAllTenants_EmptyWhenNoTenantsTable(t *testing.T) {
	ctx := context.Background()
	m, _ := newMigratorForTest(t)

	// Bootstrap creates the empty platform DB. No platform.tenants
	// table is created because the platform migrations FS is empty.
	if err := m.Bootstrap(ctx); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	results := m.MigrateAllTenants(ctx)
	if len(results) != 0 {
		t.Errorf("expected empty results, got %d", len(results))
	}
}

// TestMigrator_MigrateAllTenants_EmptyWhenNoPlatformDB verifies the
// other graceful-empty path: when the platform DB itself doesn't exist
// (Bootstrap never called, MIGRATOR_PLATFORM_ENABLED=false).
func TestMigrator_MigrateAllTenants_EmptyWhenNoPlatformDB(t *testing.T) {
	ctx := context.Background()
	m, platformDB := newMigratorForTest(t)

	// Don't call Bootstrap. The platform DB doesn't exist.
	if dbExists(t, ctx, platformDB) {
		t.Fatalf("setup error: platform DB %s should not exist", platformDB)
	}

	results := m.MigrateAllTenants(ctx)
	if len(results) != 0 {
		t.Errorf("expected empty results when platform DB missing, got %d", len(results))
	}
}

func TestMigrator_DropTenant_RemovesDB(t *testing.T) {
	ctx := context.Background()
	m, _ := newMigratorForTest(t)

	tenantID := uuid.New().String()
	dbName, err := tenant.DBName(tenantID)
	if err != nil {
		t.Fatalf("tenant.DBName: %v", err)
	}

	if err := m.ProvisionTenant(ctx, tenantID); err != nil {
		t.Fatalf("ProvisionTenant: %v", err)
	}
	if !dbExists(t, ctx, dbName) {
		t.Fatalf("expected tenant DB %s after provision", dbName)
	}

	if err := m.DropTenant(ctx, tenantID); err != nil {
		t.Fatalf("DropTenant: %v", err)
	}
	if dbExists(t, ctx, dbName) {
		t.Fatalf("expected tenant DB %s to be gone after drop", dbName)
	}
}

func TestMigrator_DropTenant_IdempotentOnMissingDB(t *testing.T) {
	ctx := context.Background()
	m, _ := newMigratorForTest(t)

	tenantID := uuid.New().String()
	dbName, err := tenant.DBName(tenantID)
	if err != nil {
		t.Fatalf("tenant.DBName: %v", err)
	}

	// Never provisioned; DB doesn't exist. Drop must succeed.
	if dbExists(t, ctx, dbName) {
		t.Fatalf("setup error: tenant DB %s should not exist", dbName)
	}
	if err := m.DropTenant(ctx, tenantID); err != nil {
		t.Fatalf("DropTenant on missing DB: %v", err)
	}
}

// TestMigrator_NewMigrator_RejectsNonPostgresScheme catches an obvious
// caller mistake (e.g. forgetting to use `postgres://`). Unit-test
// shaped — no testcontainer.
func TestMigrator_NewMigrator_RejectsNonPostgresScheme(t *testing.T) {
	_, err := NewMigrator("mysql://u:p@h:3306/db")
	if err == nil {
		t.Fatal("expected error for non-postgres scheme")
	}
	if !strings.Contains(err.Error(), "postgres://") {
		t.Errorf("error should mention postgres scheme, got: %v", err)
	}
}

// TestMigrator_MigrateTenant_RejectsInvalidUUID guards the validation
// path on tenant.DBName. Unit-test shaped.
func TestMigrator_MigrateTenant_RejectsInvalidUUID(t *testing.T) {
	m, err := NewMigrator("postgres://u:p@h:5432/postgres?sslmode=disable")
	if err != nil {
		t.Fatalf("NewMigrator: %v", err)
	}
	if _, err := m.MigrateTenant(context.Background(), "not-a-uuid"); err == nil {
		t.Fatal("expected error for invalid UUID")
	}
}

// Ensure errors.Is works against the migrate package's sentinels for
// future maintainers; currently unused by callers but documents the
// intent that callers can distinguish ErrNoChange if they ever need to.
var _ = errors.Is
