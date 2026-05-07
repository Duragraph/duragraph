package cmd

import (
	"context"
	"fmt"
	"log"

	"github.com/duragraph/duragraph/internal/domain/tenant"
	"github.com/duragraph/duragraph/internal/infrastructure/monitoring"
	"github.com/duragraph/duragraph/internal/infrastructure/persistence/postgres"
)

// bootstrapTenantMaxList caps the per-tenant gauge bootstrap to a sane
// upper bound. At MVP scale (well under 10k approved tenants per
// engine) this is effectively "list everything".
const bootstrapTenantMaxList = 10000

// bootstrapTenantMetrics seeds the per-tenant assistants_total and
// threads_total gauges from the database at engine startup.
//
// Why: those gauges are otherwise driven only by the Inc/Dec calls in
// the CreateAssistant/DeleteAssistant + CreateThread/DeleteThread
// command handlers. Without this seed, on every engine restart the
// gauges start at 0 and the first delete after restart can produce a
// negative value — gauge value after restart is then
// `0 + (creates - deletes) since restart`, not the true total.
//
// Approach: list approved tenants from the platform DB, then for each
// approved tenant open a one-shot pgxpool against the tenant DB,
// SELECT count(*) on assistants and threads, and Set() the gauge
// directly via WithLabelValues. Pools are closed before moving on so
// bootstrap doesn't accumulate connections.
//
// Per-tenant errors are logged and skipped: a single missing/broken
// tenant DB must NOT block engine startup. Aggregate failures are
// surfaced in the return value only when listing approved tenants
// itself fails — the function logs and returns nil in that case so
// the engine still comes up.
//
// Multi-replica deployments would need a separate reconciliation
// strategy (e.g. periodic resync from DB) — out of scope for v1.
func bootstrapTenantMetrics(
	ctx context.Context,
	dbCfg postgres.Config,
	tenantRepo tenant.Repository,
	metrics *monitoring.Metrics,
	logger *log.Logger,
) error {
	if metrics == nil || tenantRepo == nil {
		return nil
	}
	if logger == nil {
		logger = log.Default()
	}

	tenants, err := tenantRepo.ListByStatus(ctx, tenant.StatusApproved, bootstrapTenantMaxList, 0)
	if err != nil {
		logger.Printf("metrics bootstrap: list approved tenants failed: %v (gauges remain at zero)", err)
		return nil
	}
	if len(tenants) == 0 {
		logger.Println("metrics bootstrap: no approved tenants; nothing to seed")
		return nil
	}

	seeded := 0
	for _, t := range tenants {
		if ctx.Err() != nil {
			logger.Printf("metrics bootstrap: ctx cancelled after %d/%d tenants", seeded, len(tenants))
			return ctx.Err()
		}
		if err := seedTenantGauges(ctx, dbCfg, t.ID(), metrics, logger); err != nil {
			logger.Printf("metrics bootstrap: tenant %s skipped: %v", t.ID(), err)
			continue
		}
		seeded++
	}
	fmt.Printf("✅ Tenant metrics bootstrap complete (%d/%d tenants seeded)\n", seeded, len(tenants))
	return nil
}

// seedTenantGauges opens a one-shot pool against a single tenant DB,
// counts assistants + threads, and writes the result to the
// per-tenant gauges. The pool is closed before return so bootstrap
// holds zero connections after this function exits.
func seedTenantGauges(
	ctx context.Context,
	dbCfg postgres.Config,
	tenantID string,
	metrics *monitoring.Metrics,
	logger *log.Logger,
) error {
	dbName, err := tenant.DBName(tenantID)
	if err != nil {
		return fmt.Errorf("derive db name: %w", err)
	}

	tenantCfg := dbCfg
	tenantCfg.Database = dbName

	pool, err := postgres.NewPool(ctx, tenantCfg)
	if err != nil {
		return fmt.Errorf("open tenant pool (db=%s): %w", dbName, err)
	}
	defer pool.Close()

	var assistantCount int64
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM assistants`).Scan(&assistantCount); err != nil {
		// A tenant DB without the expected table indicates an
		// out-of-band migration state — log and continue rather than
		// abort startup.
		return fmt.Errorf("count assistants (db=%s): %w", dbName, err)
	}
	metrics.AssistantsTotal.WithLabelValues(tenantID).Set(float64(assistantCount))

	var threadCount int64
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM threads`).Scan(&threadCount); err != nil {
		return fmt.Errorf("count threads (db=%s): %w", dbName, err)
	}
	metrics.ThreadsTotal.WithLabelValues(tenantID).Set(float64(threadCount))

	logger.Printf("metrics bootstrap: tenant %s seeded (assistants=%d threads=%d)", tenantID, assistantCount, threadCount)
	return nil
}
