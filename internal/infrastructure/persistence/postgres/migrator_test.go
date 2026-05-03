//go:build integration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"testing/fstest"
	"time"

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
// and checks information_schema for tableName in the public schema.
// Used to verify that tenant migrations actually created the expected
// schema objects.
func tableExists(t *testing.T, ctx context.Context, dbName, tableName string) bool {
	t.Helper()
	return tableExistsInSchema(t, ctx, dbName, "public", tableName)
}

// tableExistsInSchema is the schema-aware variant of tableExists. The
// platform DB places its objects under the `platform` schema (per the
// migrator's `SELECT FROM platform.tenants` query), so the platform
// migrations need a schema-qualified existence check.
func tableExistsInSchema(t *testing.T, ctx context.Context, dbName, schema, tableName string) bool {
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
			WHERE table_schema=$1 AND table_name=$2
		)`, schema, tableName,
	).Scan(&exists); err != nil {
		t.Fatalf("check table %s.%s.%s: %v", dbName, schema, tableName, err)
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

// platformConn opens a direct pgx connection to the platform DB for
// tests that need to insert or assert against `platform.users` /
// `platform.tenants` directly. Caller closes via the returned cleanup.
func platformConn(t *testing.T, ctx context.Context, platformDB string) *pgx.Conn {
	t.Helper()
	_, dsn := sharedContainer(t)
	conn, err := pgx.Connect(ctx, adminURLForDB(t, dsn, platformDB))
	if err != nil {
		t.Fatalf("connect platform DB %s: %v", platformDB, err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	return conn
}

// insertUser inserts a row into platform.users with overridable
// status/role/email and returns the generated id. Defaults produce a
// valid `pending`/`user`/google user.
type userOpts struct {
	id            *string
	oauthProvider string
	oauthID       string
	email         string
	role          string
	status        string
}

func insertUser(t *testing.T, ctx context.Context, conn *pgx.Conn, o userOpts) string {
	t.Helper()
	if o.oauthProvider == "" {
		o.oauthProvider = "google"
	}
	if o.oauthID == "" {
		o.oauthID = "oauth-" + uuid.New().String()
	}
	if o.email == "" {
		o.email = uuid.New().String() + "@example.com"
	}
	if o.role == "" {
		o.role = "user"
	}
	if o.status == "" {
		o.status = "pending"
	}
	var id string
	if o.id != nil {
		id = *o.id
		_, err := conn.Exec(ctx, `
			INSERT INTO platform.users (id, oauth_provider, oauth_id, email, role, status)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, id, o.oauthProvider, o.oauthID, o.email, o.role, o.status)
		if err != nil {
			t.Fatalf("insert user: %v", err)
		}
		return id
	}
	err := conn.QueryRow(ctx, `
		INSERT INTO platform.users (oauth_provider, oauth_id, email, role, status)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id::text
	`, o.oauthProvider, o.oauthID, o.email, o.role, o.status).Scan(&id)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
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

// TestMigrator_HasMigrations_DetectsEmptyDir preserves coverage of the
// empty-FS short-circuit branch in MigratePlatform. The previous
// integration test `Bootstrap_HandlesEmptyPlatformMigrationsDir`
// relied on the embedded migrations/platform/ being empty — once
// feat/platform-db-init lands real .up.sql files the branch can no
// longer be exercised through Bootstrap, so this is now exercised
// directly against `hasMigrations` with a synthetic in-memory FS.
func TestMigrator_HasMigrations_DetectsEmptyDir(t *testing.T) {
	// Empty (only a non-SQL file): hasMigrations must report false.
	emptyFS := fstest.MapFS{
		"migrations/platform/README.md": &fstest.MapFile{Data: []byte("placeholder")},
	}
	any, err := hasMigrations(emptyFS, "migrations/platform")
	if err != nil {
		t.Fatalf("hasMigrations(empty): %v", err)
	}
	if any {
		t.Errorf("hasMigrations(empty) = true, want false")
	}

	// Populated with at least one .up.sql: must report true.
	populatedFS := fstest.MapFS{
		"migrations/platform/README.md":         &fstest.MapFile{Data: []byte("notes")},
		"migrations/platform/001_init.up.sql":   &fstest.MapFile{Data: []byte("CREATE TABLE x();")},
		"migrations/platform/001_init.down.sql": &fstest.MapFile{Data: []byte("DROP TABLE x;")},
	}
	any, err = hasMigrations(populatedFS, "migrations/platform")
	if err != nil {
		t.Fatalf("hasMigrations(populated): %v", err)
	}
	if !any {
		t.Errorf("hasMigrations(populated) = false, want true")
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

// TestMigrator_MigratePlatform_AppliesAllPlatformMigrations replaces
// the previous `Bootstrap_HandlesEmptyPlatformMigrationsDir` test now
// that real platform migrations exist. Verifies that Bootstrap creates
// the DB and runs platform/* migrations end-to-end.
func TestMigrator_MigratePlatform_AppliesAllPlatformMigrations(t *testing.T) {
	ctx := context.Background()
	m, platformDB := newMigratorForTest(t)

	if err := m.Bootstrap(ctx); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if !dbExists(t, ctx, platformDB) {
		t.Fatalf("expected platform DB %s to exist", platformDB)
	}

	for _, table := range []string{"users", "tenants", "audit_log"} {
		if !tableExistsInSchema(t, ctx, platformDB, "platform", table) {
			t.Errorf("expected table platform.%s in %s, not found", table, platformDB)
		}
	}
}

// TestMigrator_PlatformMigrations_Idempotent verifies that re-running
// Bootstrap (and therefore MigratePlatform) is a no-op against an
// already-migrated platform DB.
func TestMigrator_PlatformMigrations_Idempotent(t *testing.T) {
	ctx := context.Background()
	m, platformDB := newMigratorForTest(t)

	if err := m.Bootstrap(ctx); err != nil {
		t.Fatalf("first Bootstrap: %v", err)
	}
	v1, dirty := migrationVersion(t, ctx, platformDB)
	if dirty {
		t.Fatalf("schema_migrations dirty after first Bootstrap")
	}

	if err := m.Bootstrap(ctx); err != nil {
		t.Fatalf("second Bootstrap: %v", err)
	}
	v2, dirty := migrationVersion(t, ctx, platformDB)
	if dirty {
		t.Fatalf("schema_migrations dirty after second Bootstrap")
	}
	if v1 != v2 {
		t.Errorf("platform migration version changed across idempotent runs: v1=%d v2=%d", v1, v2)
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

// TestMigrator_MigrateAllTenants_EmptyWhenNoApprovedTenants verifies
// the empty-result path now that platform migrations create the
// `platform.tenants` table: when the table exists but has no
// status='approved' rows, MigrateAllTenants returns no results.
//
// Note: the previous test `EmptyWhenNoTenantsTable` exercised the
// 42P01 (undefined_table) error path inside listApprovedTenants. That
// path is now harder to trigger because Bootstrap creates the table —
// see TestMigrator_MigrateAllTenants_EmptyWhenTenantsTableMissing for
// the explicit coverage.
func TestMigrator_MigrateAllTenants_EmptyWhenNoApprovedTenants(t *testing.T) {
	ctx := context.Background()
	m, _ := newMigratorForTest(t)

	if err := m.Bootstrap(ctx); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	results := m.MigrateAllTenants(ctx)
	if len(results) != 0 {
		t.Errorf("expected empty results, got %d", len(results))
	}
}

// TestMigrator_MigrateAllTenants_EmptyWhenTenantsTableMissing
// preserves coverage of the 42P01 fall-through branch by manually
// dropping `platform.tenants` after Bootstrap to simulate a partially
// initialized platform DB (e.g. mid-migration). The migrator must not
// panic and must return no results.
func TestMigrator_MigrateAllTenants_EmptyWhenTenantsTableMissing(t *testing.T) {
	ctx := context.Background()
	m, platformDB := newMigratorForTest(t)

	if err := m.Bootstrap(ctx); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	// Drop platform.tenants directly to simulate the pre-init state.
	conn := platformConn(t, ctx, platformDB)
	if _, err := conn.Exec(ctx, `DROP TABLE platform.tenants`); err != nil {
		t.Fatalf("drop platform.tenants: %v", err)
	}

	results := m.MigrateAllTenants(ctx)
	if len(results) != 0 {
		t.Errorf("expected empty results when tenants table missing, got %d", len(results))
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

// TestMigrator_MigrateAllTenants_FindsApprovedTenants is the
// end-to-end success path: insert a user + tenant in `platform.users`
// + `platform.tenants` with status='approved', then call
// MigrateAllTenants and assert the tenant is included in the results.
// This proves the schema-qualified `platform.tenants` query connects.
func TestMigrator_MigrateAllTenants_FindsApprovedTenants(t *testing.T) {
	ctx := context.Background()
	m, platformDB := newMigratorForTest(t)

	if err := m.Bootstrap(ctx); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	// Provision a real tenant DB (so MigrateTenant succeeds against it).
	tenantID := uuid.New().String()
	dbName, err := tenant.DBName(tenantID)
	if err != nil {
		t.Fatalf("tenant.DBName: %v", err)
	}
	t.Cleanup(func() { dropDatabase(t, context.Background(), dbName) })
	if err := m.ProvisionTenant(ctx, tenantID); err != nil {
		t.Fatalf("ProvisionTenant: %v", err)
	}

	// Now register the tenant in platform.tenants with status='approved'.
	conn := platformConn(t, ctx, platformDB)
	userID := insertUser(t, ctx, conn, userOpts{status: "approved"})
	_, err = conn.Exec(ctx, `
		INSERT INTO platform.tenants (id, user_id, db_name, status, schema_version, provisioned_at)
		VALUES ($1, $2, $3, 'approved', 1, NOW())
	`, tenantID, userID, dbName)
	if err != nil {
		t.Fatalf("insert tenant: %v", err)
	}

	results := m.MigrateAllTenants(ctx)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].TenantID != tenantID {
		t.Errorf("result tenant id = %q, want %q", results[0].TenantID, tenantID)
	}
	if results[0].Err != nil {
		t.Errorf("expected no error on already-migrated tenant, got: %v", results[0].Err)
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

// =============================================================================
// platform.users / platform.tenants CHECK + UNIQUE constraint coverage
// =============================================================================

// TestPlatformDB_UsersConstraints exercises the column-level CHECK
// constraints (oauth_provider, role, status) and the two UNIQUE
// constraints (email; (oauth_provider, oauth_id)) on platform.users.
func TestPlatformDB_UsersConstraints(t *testing.T) {
	ctx := context.Background()
	m, platformDB := newMigratorForTest(t)
	if err := m.Bootstrap(ctx); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	conn := platformConn(t, ctx, platformDB)

	// Baseline: a valid user inserts cleanly.
	insertUser(t, ctx, conn, userOpts{})

	// Invalid role.
	_, err := conn.Exec(ctx, `
		INSERT INTO platform.users (oauth_provider, oauth_id, email, role)
		VALUES ('google', 'oid-x', 'bad-role@example.com', 'superadmin')
	`)
	if err == nil {
		t.Errorf("expected CHECK violation for role='superadmin', got nil")
	}

	// Invalid status.
	_, err = conn.Exec(ctx, `
		INSERT INTO platform.users (oauth_provider, oauth_id, email, status)
		VALUES ('google', 'oid-y', 'bad-status@example.com', 'banned')
	`)
	if err == nil {
		t.Errorf("expected CHECK violation for status='banned', got nil")
	}

	// Invalid oauth_provider.
	_, err = conn.Exec(ctx, `
		INSERT INTO platform.users (oauth_provider, oauth_id, email)
		VALUES ('facebook', 'oid-z', 'bad-provider@example.com')
	`)
	if err == nil {
		t.Errorf("expected CHECK violation for oauth_provider='facebook', got nil")
	}

	// Duplicate email rejection.
	insertUser(t, ctx, conn, userOpts{email: "dup@example.com", oauthID: "oid-a"})
	_, err = conn.Exec(ctx, `
		INSERT INTO platform.users (oauth_provider, oauth_id, email)
		VALUES ('github', 'oid-b', 'dup@example.com')
	`)
	if err == nil {
		t.Errorf("expected UNIQUE violation for duplicate email, got nil")
	}

	// Duplicate (oauth_provider, oauth_id) rejection.
	insertUser(t, ctx, conn, userOpts{oauthProvider: "github", oauthID: "shared-oid"})
	_, err = conn.Exec(ctx, `
		INSERT INTO platform.users (oauth_provider, oauth_id, email)
		VALUES ('github', 'shared-oid', 'other-user@example.com')
	`)
	if err == nil {
		t.Errorf("expected UNIQUE violation on (oauth_provider, oauth_id), got nil")
	}
}

// validTenantInsert performs the canonical insert path: derives db_name
// from id and writes status='pending' (no schema_version /
// provisioned_at required). Returns (id, db_name).
func validTenantInsert(t *testing.T, ctx context.Context, conn *pgx.Conn, userID string) (string, string) {
	t.Helper()
	id := uuid.New().String()
	dbName := "tenant_" + strings.ReplaceAll(id, "-", "")
	_, err := conn.Exec(ctx, `
		INSERT INTO platform.tenants (id, user_id, db_name, status)
		VALUES ($1, $2, $3, 'pending')
	`, id, userID, dbName)
	if err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	return id, dbName
}

// TestPlatformDB_TenantsDerivationCheck verifies the table-level
// `tenants_db_name_derived_from_id` CHECK rejects rows where db_name
// is not equal to 'tenant_' || replace(id::text, '-', ”).
func TestPlatformDB_TenantsDerivationCheck(t *testing.T) {
	ctx := context.Background()
	m, platformDB := newMigratorForTest(t)
	if err := m.Bootstrap(ctx); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	conn := platformConn(t, ctx, platformDB)
	userID := insertUser(t, ctx, conn, userOpts{})

	// Mismatched db_name (different UUID under the prefix). The format
	// regex passes (32 hex chars) but the derivation CHECK fails.
	id := uuid.New().String()
	otherDBName := "tenant_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	_, err := conn.Exec(ctx, `
		INSERT INTO platform.tenants (id, user_id, db_name, status)
		VALUES ($1, $2, $3, 'pending')
	`, id, userID, otherDBName)
	if err == nil {
		t.Errorf("expected CHECK violation tenants_db_name_derived_from_id, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "tenants_db_name_derived_from_id") {
		// Postgres does include the constraint name in its error
		// detail; if it doesn't, the test still catches the rejection
		// but we log to surface the change.
		t.Logf("derivation CHECK rejection (constraint name not in msg): %v", err)
	}
}

// TestPlatformDB_TenantsApprovedRequiresSchemaVersion verifies the
// `tenants_approved_requires_schema_version` table-level CHECK.
func TestPlatformDB_TenantsApprovedRequiresSchemaVersion(t *testing.T) {
	ctx := context.Background()
	m, platformDB := newMigratorForTest(t)
	if err := m.Bootstrap(ctx); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	conn := platformConn(t, ctx, platformDB)
	userID := insertUser(t, ctx, conn, userOpts{status: "approved"})

	id := uuid.New().String()
	dbName := "tenant_" + strings.ReplaceAll(id, "-", "")
	_, err := conn.Exec(ctx, `
		INSERT INTO platform.tenants (id, user_id, db_name, status, provisioned_at)
		VALUES ($1, $2, $3, 'approved', NOW())
	`, id, userID, dbName)
	if err == nil {
		t.Errorf("expected CHECK violation for approved tenant with NULL schema_version, got nil")
	}
}

// TestPlatformDB_TenantsApprovedRequiresProvisionedAt verifies the
// `tenants_approved_requires_provisioned_at` table-level CHECK.
func TestPlatformDB_TenantsApprovedRequiresProvisionedAt(t *testing.T) {
	ctx := context.Background()
	m, platformDB := newMigratorForTest(t)
	if err := m.Bootstrap(ctx); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	conn := platformConn(t, ctx, platformDB)
	userID := insertUser(t, ctx, conn, userOpts{status: "approved"})

	id := uuid.New().String()
	dbName := "tenant_" + strings.ReplaceAll(id, "-", "")
	_, err := conn.Exec(ctx, `
		INSERT INTO platform.tenants (id, user_id, db_name, status, schema_version)
		VALUES ($1, $2, $3, 'approved', 1)
	`, id, userID, dbName)
	if err == nil {
		t.Errorf("expected CHECK violation for approved tenant with NULL provisioned_at, got nil")
	}
}

// TestPlatformDB_TenantsFailureReasonGuard verifies the
// `tenants_failure_reason_only_when_failed` CHECK: failure_reason can
// only be non-NULL when status='provisioning_failed'.
func TestPlatformDB_TenantsFailureReasonGuard(t *testing.T) {
	ctx := context.Background()
	m, platformDB := newMigratorForTest(t)
	if err := m.Bootstrap(ctx); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	conn := platformConn(t, ctx, platformDB)
	userID := insertUser(t, ctx, conn, userOpts{})

	id := uuid.New().String()
	dbName := "tenant_" + strings.ReplaceAll(id, "-", "")
	_, err := conn.Exec(ctx, `
		INSERT INTO platform.tenants (id, user_id, db_name, status, failure_reason)
		VALUES ($1, $2, $3, 'pending', 'should not be allowed when pending')
	`, id, userID, dbName)
	if err == nil {
		t.Errorf("expected CHECK violation for failure_reason on non-failed tenant, got nil")
	}

	// Sanity: the same row with status='provisioning_failed' is allowed.
	_, err = conn.Exec(ctx, `
		INSERT INTO platform.tenants (id, user_id, db_name, status, failure_reason)
		VALUES ($1, $2, $3, 'provisioning_failed', 'CREATE DATABASE failed')
	`, id, userID, dbName)
	if err != nil {
		t.Errorf("provisioning_failed + failure_reason should be allowed, got: %v", err)
	}
}

// TestPlatformDB_TenantsUserIdUnique verifies the 1:1 user↔tenant
// constraint via tenants_user_id_unique.
func TestPlatformDB_TenantsUserIdUnique(t *testing.T) {
	ctx := context.Background()
	m, platformDB := newMigratorForTest(t)
	if err := m.Bootstrap(ctx); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	conn := platformConn(t, ctx, platformDB)
	userID := insertUser(t, ctx, conn, userOpts{})

	// First tenant for this user inserts cleanly.
	validTenantInsert(t, ctx, conn, userID)

	// Second tenant for the same user is rejected.
	id := uuid.New().String()
	dbName := "tenant_" + strings.ReplaceAll(id, "-", "")
	_, err := conn.Exec(ctx, `
		INSERT INTO platform.tenants (id, user_id, db_name, status)
		VALUES ($1, $2, $3, 'pending')
	`, id, userID, dbName)
	if err == nil {
		t.Errorf("expected UNIQUE violation on tenants_user_id_unique, got nil")
	}
}

// TestPlatformDB_TenantsDBNameFormatCheck verifies the column-level
// regex check on db_name (independent from the derivation check).
func TestPlatformDB_TenantsDBNameFormatCheck(t *testing.T) {
	ctx := context.Background()
	m, platformDB := newMigratorForTest(t)
	if err := m.Bootstrap(ctx); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	conn := platformConn(t, ctx, platformDB)
	userID := insertUser(t, ctx, conn, userOpts{})

	id := uuid.New().String()
	// Wrong format — uppercase hex would violate the regex
	// `^tenant_[a-f0-9]{32}$`.
	badDBName := "tenant_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	_, err := conn.Exec(ctx, `
		INSERT INTO platform.tenants (id, user_id, db_name, status)
		VALUES ($1, $2, $3, 'pending')
	`, id, userID, badDBName)
	if err == nil {
		t.Errorf("expected CHECK violation for invalid db_name format, got nil")
	}
}

// TestPlatformDB_AuditLogAppendOnly is a smoke test that the audit_log
// table accepts inserts shaped like the spec's UserSignedUp /
// TenantApproved events. Designed to fail loud if a future migration
// accidentally drops a required column.
func TestPlatformDB_AuditLogAppendOnly(t *testing.T) {
	ctx := context.Background()
	m, platformDB := newMigratorForTest(t)
	if err := m.Bootstrap(ctx); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	conn := platformConn(t, ctx, platformDB)

	userID := insertUser(t, ctx, conn, userOpts{})

	// Insert a sample user.signed_up audit row.
	_, err := conn.Exec(ctx, `
		INSERT INTO platform.audit_log (event_type, aggregate_type, aggregate_id, payload, occurred_at)
		VALUES ('user.signed_up', 'user', $1, '{"email":"new@example.com"}'::jsonb, $2)
	`, userID, time.Now().UTC())
	if err != nil {
		t.Fatalf("insert audit_log row: %v", err)
	}

	// Insert a sample tenant.approved with actor + reason.
	tenantID := uuid.New().String()
	_, err = conn.Exec(ctx, `
		INSERT INTO platform.audit_log (
			event_type, aggregate_type, aggregate_id, actor_user_id,
			payload, reason, ip_address, user_agent, occurred_at
		)
		VALUES (
			'tenant.approved', 'tenant', $1, $2,
			'{"db_name":"tenant_x"}'::jsonb, 'manual review', '127.0.0.1', 'test-agent', $3
		)
	`, tenantID, userID, time.Now().UTC())
	if err != nil {
		t.Fatalf("insert audit_log row with full fields: %v", err)
	}
}

// Ensure errors.Is works against the migrate package's sentinels for
// future maintainers; currently unused by callers but documents the
// intent that callers can distinguish ErrNoChange if they ever need to.
var _ = errors.Is
