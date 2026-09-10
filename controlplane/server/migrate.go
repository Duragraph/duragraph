// Package server is the Layer-4 composition root of the control-plane
// rebuild per STRUCTURE.md. It wires the migrations, the
// endpoints.Server (tenant + platform pgxpool), the NATS relay, and
// the Echo router into a single runnable binary with graceful shutdown.
//
// Source of truth for the assembly: spec/models/system-architecture.d2
// (api → endpoints → relay → nats, plus the platform-provisioner side
// effect) and spec/models/d2/relay.d2 + nats.d2. The d2 stays the
// human spec — this file is its machine form.
package server

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/duragraph/duragraph/controlplane/db"
)

// migrationsTable records which migrations have run.
//
// Without it every restart re-executed 001, hit "relation already
// exists" on a plain CREATE TABLE, and the process refused to start. The
// server could be started exactly once per database. No test caught it
// because every test either sets Migrate:false or gets a fresh container,
// which is precisely the shape of bug that only appears in production.
const migrationsTable = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version     TEXT PRIMARY KEY,
    applied_at  TIMESTAMPTZ NOT NULL DEFAULT now()
)`

// ApplyMigrations runs every *.up.sql under dir against pool. It is a
// thin wrapper over ApplyMigrationsFS for callers that genuinely have a
// directory — tests pointing at a checkout, or an operator overriding
// MigrateDir. Production uses the embedded tree.
func ApplyMigrations(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("migrate: stat %q: %w", dir, err)
	}
	return ApplyMigrationsFS(ctx, pool, os.DirFS(dir))
}

// ApplyMigrationsFS runs every *.up.sql at the root of fsys against pool
// in lexical order (migrations are zero-padded NN_name.up.sql, so lexical
// equals numeric).
//
// It is IDEMPOTENT, which the previous version was not. Each migration is
// applied inside its own transaction together with the row recording it,
// so a migration and the fact of that migration commit or roll back as
// one — a crash between the two cannot leave the database claiming work
// it did not do, or hide work it did. Already-recorded versions are
// skipped, so a restart is a no-op rather than a fatal error.
//
// One transaction per migration rather than one for all of them: Postgres
// rolls back the entire transaction on any error, so a single wrapping
// transaction would discard successful earlier migrations and re-run them
// next boot. Per-migration means a failure at 004 leaves 001-003 durably
// applied and recorded, and the next start resumes at 004.
func ApplyMigrationsFS(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS) error {
	if pool == nil {
		return errors.New("migrate: nil pool")
	}
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return fmt.Errorf("migrate: read migrations: %w", err)
	}
	var ups []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".up.sql") {
			ups = append(ups, e.Name())
		}
	}
	sort.Strings(ups)
	if len(ups) == 0 {
		return errors.New("migrate: no *.up.sql migrations found")
	}

	if _, err := pool.Exec(ctx, migrationsTable); err != nil {
		return fmt.Errorf("migrate: ensure schema_migrations: %w", err)
	}

	applied, err := appliedVersions(ctx, pool)
	if err != nil {
		return err
	}

	for _, name := range ups {
		version := strings.TrimSuffix(name, ".up.sql")
		if applied[version] {
			continue
		}
		sqlBytes, err := fs.ReadFile(fsys, name)
		if err != nil {
			return fmt.Errorf("migrate: read %s: %w", name, err)
		}
		if err := applyOne(ctx, pool, version, string(sqlBytes)); err != nil {
			return err
		}
	}
	return nil
}

// appliedVersions reads the set of migrations already recorded.
func appliedVersions(ctx context.Context, pool *pgxpool.Pool) (map[string]bool, error) {
	rows, err := pool.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("migrate: read schema_migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("migrate: scan schema_migrations: %w", err)
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("migrate: iterate schema_migrations: %w", err)
	}
	return applied, nil
}

// applyOne runs a single migration and records it in the same
// transaction, so the schema change and the claim that it happened share
// one fate.
func applyOne(ctx context.Context, pool *pgxpool.Pool, version, sql string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("migrate: begin %s: %w", version, err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once committed

	if _, err := tx.Exec(ctx, sql); err != nil {
		return fmt.Errorf("migrate: apply %s: %w", version, err)
	}
	// ON CONFLICT DO NOTHING covers two servers booting against the same
	// fresh database at once. The loser's INSERT is a no-op rather than a
	// unique-violation that would crash an otherwise healthy process.
	if _, err := tx.Exec(ctx,
		`INSERT INTO schema_migrations (version) VALUES ($1) ON CONFLICT DO NOTHING`,
		version); err != nil {
		return fmt.Errorf("migrate: record %s: %w", version, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("migrate: commit %s: %w", version, err)
	}
	return nil
}

// migrationRoot resolves where migrations come from.
//
// The embedded tree is the default so the server works from any working
// directory — the shipped binary has no source checkout to read. An
// explicit MigrateDir still wins, for an operator who needs to apply SQL
// this build does not carry.
func migrationRoot(dir string) (fs.FS, error) {
	if dir == "" {
		return db.FS(), nil
	}
	if _, err := os.Stat(dir); err != nil {
		return nil, fmt.Errorf("MigrateDir %q: %w", dir, err)
	}
	return os.DirFS(dir), nil
}
