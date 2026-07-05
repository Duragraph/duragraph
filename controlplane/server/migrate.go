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
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ApplyMigrations runs every *.up.sql in dir against pool, in lexical
// order (the rebuild's migrations are zero-padded NN_name.up.sql so
// lexical == numeric). Idempotent only insofar as the SQL itself is —
// migrations use CREATE TABLE (no IF NOT EXISTS) so a second run on a
// non-empty DB errors out, which is the correct behaviour for a
// first-boot vs upgrade decision (the relay/event-store doesn't yet
// track schema_version).
//
// Mirrors the applyTenantMigrations helpers in the endpoints +
// controlplane/nats test files; this is the production version that
// runs from disk paths resolved at startup.
func ApplyMigrations(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	if pool == nil {
		return errors.New("migrate: nil pool")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("migrate: read dir %q: %w", dir, err)
	}
	var ups []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".up.sql") {
			ups = append(ups, e.Name())
		}
	}
	sort.Strings(ups)
	if len(ups) == 0 {
		return fmt.Errorf("migrate: no *.up.sql migrations found in %q", dir)
	}
	for _, name := range ups {
		sqlBytes, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("migrate: read %s: %w", name, err)
		}
		if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
			return fmt.Errorf("migrate: apply %s: %w", name, err)
		}
	}
	return nil
}
