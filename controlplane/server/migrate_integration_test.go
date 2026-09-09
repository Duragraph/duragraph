package server_test

import (
	"context"
	"fmt"
	"io/fs"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/duragraph/duragraph/controlplane/db"
	dgserver "github.com/duragraph/duragraph/controlplane/server"
)

// freshDatabase creates an empty database on the test container and
// returns a DSN for it. Migrations have to be exercised against a
// database that has never seen them, which the shared TestMain one has.
func freshDatabase(t *testing.T, ctx context.Context) string {
	t.Helper()

	admin, err := pgxpool.New(ctx, tenantDSN)
	if err != nil {
		t.Fatalf("admin pool: %v", err)
	}
	defer admin.Close()

	name := fmt.Sprintf("mig_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("create database: %v", err)
	}

	u, err := url.Parse(tenantDSN)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	u.Path = "/" + name
	return u.String()
}

func tenantMigrations(t *testing.T) fs.FS {
	t.Helper()
	sub, err := fs.Sub(db.FS(), "tenant")
	if err != nil {
		t.Fatalf("tenant subtree: %v", err)
	}
	return sub
}

// TestMigrationsAreIdempotent is the regression test for a bug that made
// the rebuilt control plane unable to restart.
//
// Migrations use plain CREATE TABLE and nothing recorded which had run,
// so the second boot against the same database re-executed 001, hit
// "relation already exists", and the process refused to start. The server
// could be started exactly once per database. Every test set
// Migrate:false or got a fresh container, so nothing caught it — the
// failure was reachable only by restarting a real deployment.
func TestMigrationsAreIdempotent(t *testing.T) {
	ctx := context.Background()
	dsn := freshDatabase(t, ctx)

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()

	migrations := tenantMigrations(t)

	if err := dgserver.ApplyMigrationsFS(ctx, pool, migrations); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	var afterFirst int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&afterFirst); err != nil {
		t.Fatal(err)
	}
	if afterFirst == 0 {
		t.Fatal("first apply recorded no migrations")
	}

	// THE BUG: this second call used to fail with "relation already exists".
	if err := dgserver.ApplyMigrationsFS(ctx, pool, migrations); err != nil {
		t.Fatalf("second apply must be a no-op, got: %v", err)
	}

	var afterSecond int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&afterSecond); err != nil {
		t.Fatal(err)
	}
	if afterSecond != afterFirst {
		t.Errorf("second apply changed the recorded set: %d -> %d", afterFirst, afterSecond)
	}

	// A third, to catch anything that only breaks once state exists.
	if err := dgserver.ApplyMigrationsFS(ctx, pool, migrations); err != nil {
		t.Fatalf("third apply: %v", err)
	}

	// The schema is really there, not merely claimed.
	for _, table := range []string{"events", "outbox", "assistants", "threads", "runs"} {
		var exists bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM information_schema.tables
			                WHERE table_schema='public' AND table_name=$1)`,
			table).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Errorf("table %q missing after migrations", table)
		}
	}
}

// TestMigrationsRecordEveryVersion pins the recorded name to the file
// name. If a migration were recorded under a different key than the one
// the skip-check computes, every boot would re-run it — the original bug
// wearing a bookkeeping table as a disguise.
func TestMigrationsRecordEveryVersion(t *testing.T) {
	ctx := context.Background()
	dsn := freshDatabase(t, ctx)

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()

	migrations := tenantMigrations(t)
	if err := dgserver.ApplyMigrationsFS(ctx, pool, migrations); err != nil {
		t.Fatalf("apply: %v", err)
	}

	entries, err := fs.ReadDir(migrations, ".")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".up.sql") {
			want[strings.TrimSuffix(e.Name(), ".up.sql")] = true
		}
	}
	if len(want) == 0 {
		t.Fatal("no migrations found in the embedded tree")
	}

	rows, err := pool.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]bool{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		got[v] = true
	}
	for v := range want {
		if !got[v] {
			t.Errorf("migration %q ran but was not recorded", v)
		}
	}
	for v := range got {
		if !want[v] {
			t.Errorf("recorded %q which is not a migration file", v)
		}
	}
}

// TestEmbeddedMigrationsAreComplete guards the go:embed directive. A
// mistyped pattern yields an empty tree that fails only at deploy time,
// so assert the shape here where it is cheap.
func TestEmbeddedMigrationsAreComplete(t *testing.T) {
	for _, dir := range []string{"tenant", "platform"} {
		sub, err := fs.Sub(db.FS(), dir)
		if err != nil {
			t.Fatalf("%s subtree: %v", dir, err)
		}
		entries, err := fs.ReadDir(sub, ".")
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}
		var ups int
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".up.sql") {
				ups++
			}
		}
		if ups == 0 {
			t.Errorf("embedded %s tree carries no *.up.sql migrations", dir)
		}
	}
}

// TestServerMigratesOnBootTwice is the end-to-end form: the composition
// root itself, with Migrate:true, started twice against one database.
// This is what a restart does, and what used to fail.
func TestServerMigratesOnBootTwice(t *testing.T) {
	ctx := context.Background()
	dsn := freshDatabase(t, ctx)

	for i := 1; i <= 2; i++ {
		srv, err := dgserver.New(ctx, dgserver.Config{
			Addr:      "127.0.0.1:0",
			TenantDSN: dsn,
			Migrate:   true, // embedded tree; no MigrateDir, no CWD dependency
			// Nothing is in flight; do not sit through the default drain.
			DrainTimeout: 2 * time.Second,
		})
		if err != nil {
			t.Fatalf("boot %d: %v", i, err)
		}
		if err := srv.Shutdown(ctx); err != nil {
			t.Fatalf("boot %d shutdown: %v", i, err)
		}
	}
}
