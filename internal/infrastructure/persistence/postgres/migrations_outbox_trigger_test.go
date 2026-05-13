package postgres_test

import (
	"context"
	"testing"
)

// TestMigration013_DroppedExactlyTheRightObjects asserts the post-state
// of migration 013_drop_outbox_trigger. The migration ran at TestMain
// time (via runtime migrator), so this test is a pure state assertion
// on pg_trigger + pg_proc.
//
// Why this exists: `DROP TRIGGER IF EXISTS` silently no-ops if the
// object isn't where the migration expects it (wrong table name, wrong
// case, etc.). Without an explicit post-state check, the migration
// would "succeed" while leaving the trigger in place, AND the new
// app-side outbox write would also fire — producing duplicate rows
// caught only by the `ON CONFLICT (event_id) DO NOTHING` safety net,
// silently masking the regression.
//
// This is Tier 1 of the regression testing plan (see
// project_regression_testing_plan.md). Whenever a future migration
// drops or alters schema objects, add a sibling test asserting the
// exact post-state.
func TestMigration013_DroppedExactlyTheRightObjects(t *testing.T) {
	ctx := context.Background()

	// (1) auto_publish_to_outbox trigger must be GONE.
	var triggerCount int
	err := testPool.QueryRow(ctx, `
		SELECT COUNT(*) FROM pg_trigger
		WHERE tgname = 'auto_publish_to_outbox'
		  AND NOT tgisinternal
	`).Scan(&triggerCount)
	if err != nil {
		t.Fatalf("query pg_trigger: %v", err)
	}
	if triggerCount != 0 {
		t.Errorf("auto_publish_to_outbox trigger should be dropped, found %d copy/copies", triggerCount)
	}

	// (2) publish_event_to_outbox function must be GONE.
	var funcCount int
	err = testPool.QueryRow(ctx, `
		SELECT COUNT(*) FROM pg_proc
		WHERE proname = 'publish_event_to_outbox'
	`).Scan(&funcCount)
	if err != nil {
		t.Fatalf("query pg_proc (publish_event_to_outbox): %v", err)
	}
	if funcCount != 0 {
		t.Errorf("publish_event_to_outbox function should be dropped, found %d copy/copies", funcCount)
	}

	// (3) increment_version_on_event trigger must be PRESENT — it's
	// the stream-version bookkeeping trigger, a different concern,
	// and dropping it would silently break aggregate version tracking.
	err = testPool.QueryRow(ctx, `
		SELECT COUNT(*) FROM pg_trigger
		WHERE tgname = 'increment_version_on_event'
		  AND NOT tgisinternal
	`).Scan(&triggerCount)
	if err != nil {
		t.Fatalf("query pg_trigger (increment_version_on_event): %v", err)
	}
	if triggerCount != 1 {
		t.Errorf("increment_version_on_event trigger must remain (got %d copies, want 1)", triggerCount)
	}

	// (4) cleanup_published_outbox function must be PRESENT — the
	// CleanupWorker calls it on a schedule; dropping it would silently
	// disable outbox cleanup.
	err = testPool.QueryRow(ctx, `
		SELECT COUNT(*) FROM pg_proc
		WHERE proname = 'cleanup_published_outbox'
	`).Scan(&funcCount)
	if err != nil {
		t.Fatalf("query pg_proc (cleanup_published_outbox): %v", err)
	}
	if funcCount != 1 {
		t.Errorf("cleanup_published_outbox function must remain (got %d copies, want 1)", funcCount)
	}

	// (5) Belt-and-braces — only ONE non-internal trigger should
	// remain on the events table after migration 013. If a future
	// migration adds another trigger here, we want that author to
	// update this assertion deliberately.
	err = testPool.QueryRow(ctx, `
		SELECT COUNT(*) FROM pg_trigger
		WHERE tgrelid = 'events'::regclass
		  AND NOT tgisinternal
	`).Scan(&triggerCount)
	if err != nil {
		t.Fatalf("query events triggers: %v", err)
	}
	if triggerCount != 1 {
		var names []string
		rows, _ := testPool.Query(ctx, `
			SELECT tgname FROM pg_trigger
			WHERE tgrelid = 'events'::regclass AND NOT tgisinternal
		`)
		for rows.Next() {
			var n string
			_ = rows.Scan(&n)
			names = append(names, n)
		}
		rows.Close()
		t.Errorf("events table has %d non-internal triggers (want 1: increment_version_on_event); found: %v", triggerCount, names)
	}
}
